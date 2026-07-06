package web

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/kittylabassistant/sign-craze/internal/exectx"
)

// ServerConfig содержит зависимости и параметры сервера.
type ServerConfig struct {
	// ListenHost — bind-адрес для портов 9090 (Zashboard) и 9091 (admin REST).
	// LAN-trust-модель: должен быть IP LAN-bridge (например 192.168.1.1) или
	// loopback. Пустая строка (legacy) → ":9090"/":9091" (0.0.0.0) — небезопасно
	// в production, оставлено для совместимости с unit-тестами.
	ListenHost string

	Status     StatusReader
	Config     ConfigRW
	Ports      PortsManager
	Excludes   ExcludesManager
	DPITargets DPITargetsManager

	// Cores опционально предоставляет имя активного ядра для GET /api/cores.
	// Если nil, /api/cores отдаёт active="" — UI должен трактовать как unknown.
	Cores CoresProvider

	// Runner используется для exec'а core BinaryVersion и других read-only
	// диагностических вызовов. Если nil — версии в /api/cores не показываются.
	Runner exectx.Runner

	// RoutingUIEnabled включает третий http.Server для UI-редактора routing.
	RoutingUIEnabled bool
	// RoutingUIPort — желаемый стартовый порт (default 9092). Если занят,
	// FindFreePort пробует port+1, port+2, ..., до RoutingUIMaxAttempts раз.
	RoutingUIPort uint16
	// RoutingUIBind — bind-адрес. Default "0.0.0.0".
	RoutingUIBind string
	// RoutingUIMaxAttempts — сколько портов пробовать. Default 10.
	RoutingUIMaxAttempts int
	// RoutingUI — зависимости routing UI (paths, sing-box bin, регенератор).
	// Может быть nil — handlers вернут 503 для зависимых endpoints.
	RoutingUI *RoutingUIDeps
}

// Server — встроенный HTTP-сервер (порт 9090 Zashboard, 9091 admin API, опционально 9092 для routing UI).
type Server struct {
	zashboard *http.Server
	admin     *http.Server
	// routingUI — третий сервер для UI-редактора routing. nil если RoutingUIEnabled=false.
	routingUI         *http.Server
	routingUIPort     uint16       // фактически выбранный порт (после FindFreePort)
	routingUIListener net.Listener // зарезервированный listener; передаётся в routingUI.Serve

	cfg ServerConfig
}

// NewServer создаёт Server.
func NewServer(cfg ServerConfig) (*Server, error) {
	s := &Server{cfg: cfg}

	// Zashboard (порт 9090): Clash-совместимый API + встроенный SPA.
	// Доступ ограничивается iptables DROP на WAN_IF (не через bind).
	clashMux := http.NewServeMux()
	registerClashRoutes(clashMux, s)
	// Вариант (a): zashboard — отдельный http.Server, поэтому WriteTimeout=0
	// только для него. Стриминговые эндпоинты /traffic, /logs, /connections
	// не должны обрываться по таймауту; защита от slowloris обеспечивается
	// ReadHeaderTimeout (≤15 с). Admin (9091) оставляет WriteTimeout=30s.
	s.zashboard = &http.Server{
		Addr:              joinHostPort(cfg.ListenHost, "9090"),
		Handler:           recoverMiddleware(securityHeadersSPA(clashMux)),
		ReadHeaderTimeout: 15 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      0, // стримы: /traffic, /logs, /connections
		IdleTimeout:       90 * time.Second,
		MaxHeaderBytes:    8 * 1024,
	}

	adminMux := http.NewServeMux()
	registerAdminRoutes(adminMux, s)

	s.admin = &http.Server{
		Addr:              joinHostPort(cfg.ListenHost, "9091"),
		Handler:           recoverMiddleware(securityHeadersAdmin(adminMux)),
		ReadHeaderTimeout: 15 * time.Second, // защита от slowloris (симметрично zashboard)
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    8 * 1024,
	}

	if cfg.RoutingUIEnabled {
		bind := cfg.RoutingUIBind
		if bind == "" {
			bind = "0.0.0.0"
		}
		port := cfg.RoutingUIPort
		if port == 0 {
			port = 9092
		}
		max := cfg.RoutingUIMaxAttempts
		if max <= 0 {
			max = 10
		}
		chosen, listener, err := FindFreePort(bind, port, max)
		if err != nil {
			return nil, fmt.Errorf("routing UI listener: %w", err)
		}
		s.routingUIListener = listener
		s.routingUIPort = chosen

		routingMux := http.NewServeMux()
		registerRoutingUIRoutes(routingMux, s)
		s.routingUI = &http.Server{
			Handler:        recoverMiddleware(securityHeadersSPA(routingMux)),
			ReadTimeout:    15 * time.Second,
			WriteTimeout:   30 * time.Second,
			IdleTimeout:    60 * time.Second,
			MaxHeaderBytes: 8 * 1024,
		}
	}

	return s, nil
}

// RoutingUIPort возвращает фактически выбранный порт routing UI (после FindFreePort).
// 0 если RoutingUIEnabled=false.
func (s *Server) RoutingUIPort() uint16 { return s.routingUIPort }

// Start запускает все листенеры (Zashboard 9090, admin 9091, опционально routing UI 9092).
// Блокируется до отмены ctx или критической ошибки.
func (s *Server) Start(ctx context.Context) error {
	errCh := make(chan error, 3)

	go func() {
		slog.Info("web: Zashboard запущен", "addr", s.zashboard.Addr)
		if err := s.zashboard.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("zashboard listener: %w", err)
		}
	}()

	go func() {
		slog.Info("web: admin API запущен", "addr", s.admin.Addr)
		if err := s.admin.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("admin listener: %w", err)
		}
	}()

	if s.routingUI != nil && s.routingUIListener != nil {
		go func() {
			slog.Info("web: routing UI запущен", "addr", s.routingUIListener.Addr().String())
			if err := s.routingUI.Serve(s.routingUIListener); err != nil && err != http.ErrServerClosed {
				errCh <- fmt.Errorf("routing UI listener: %w", err)
			}
		}()
	}

	select {
	case <-ctx.Done():
		return s.Stop(context.Background())
	case err := <-errCh:
		if stopErr := s.Stop(context.Background()); stopErr != nil {
			slog.Warn("web: ошибка остановки", "err", stopErr)
		}
		return err
	}
}

// Stop выполняет graceful shutdown всех серверов (таймаут 5 с).
func (s *Server) Stop(ctx context.Context) error {
	shutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var errs []error
	if err := s.zashboard.Shutdown(shutCtx); err != nil {
		errs = append(errs, fmt.Errorf("zashboard: %w", err))
	}
	if err := s.admin.Shutdown(shutCtx); err != nil {
		errs = append(errs, fmt.Errorf("admin: %w", err))
	}
	if s.routingUI != nil {
		if err := s.routingUI.Shutdown(shutCtx); err != nil {
			errs = append(errs, fmt.Errorf("routing UI: %w", err))
		}
	}
	if len(errs) != 0 {
		return fmt.Errorf("web stop: %v", errs)
	}
	return nil
}

// cspAdmin — строгая Content-Security-Policy для admin REST (порт 9091).
// Лендинг — статический HTML без JS, поэтому inline-скрипты запрещены.
const cspAdmin = "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; script-src 'self'"

// cspSPA — смягчённая CSP для порта 9092 (Routing Editor).
// SPA может содержать inline <script> с конфигом приложения.
// Без 'unsafe-inline' SPA не инициализируется — страница белая.
// Риск минимален: контент целиком из embed.FS (compile-time),
// источник 'self'. Basic Auth в проекте отсутствует (каркас удалён в Phase 17) —
// реальный барьер доступа — WAN INPUT DROP на UI-порты (firewall) и LAN-only
// bind (см. ServerConfig.ListenHost / netif.DetectLANAddr), а не auth.
// 'unsafe-eval' нужен на случай dynamic-import-полифиллов.
// connect-src с ws:/wss: разрешает WebSocket-соединения routing UI.
const cspSPA = "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; script-src 'self' 'unsafe-inline' 'unsafe-eval'; connect-src 'self' ws: wss:"

// securityHeadersAdmin ставит защитные заголовки + строгую CSP для admin.
func securityHeadersAdmin(next http.Handler) http.Handler {
	return setSecurityHeaders(next, cspAdmin)
}

// securityHeadersSPA ставит защитные заголовки + CSP, разрешающую inline-скрипты.
func securityHeadersSPA(next http.Handler) http.Handler {
	return setSecurityHeaders(next, cspSPA)
}

func setSecurityHeaders(next http.Handler, csp string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Content-Security-Policy", csp)
		next.ServeHTTP(w, r)
	})
}

// joinHostPort возвращает host:port для http.Server.Addr. Пустой host
// сохраняет legacy-поведение (":port" = 0.0.0.0) для unit-тестов; в
// production cmd_ui подставляет конкретный LAN-IP.
func joinHostPort(host, port string) string {
	if host == "" {
		return ":" + port
	}
	return net.JoinHostPort(host, port)
}

// recoverMiddleware перехватывает паники и возвращает 500.
func recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("web: паника в обработчике", "err", rec)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
