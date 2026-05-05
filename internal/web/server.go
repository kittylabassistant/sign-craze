package web

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"
)

// adminUsername — единственный допустимый логин Basic Auth.
const adminUsername = "admin"

// nonWSWriteDeadline — таймаут записи для не-WebSocket Clash-роутов.
// Защита от slowloris: WriteTimeout всего сервера = 0 (для WS), но обычные
// HTTP-роуты не должны висеть бесконечно.
const nonWSWriteDeadline = 30 * time.Second

// wsRoutes — пути, исключённые из WriteDeadline (поток данных, может идти долго).
var wsRoutes = map[string]struct{}{
	"/traffic": {},
	"/logs":    {},
}

// ServerConfig содержит зависимости и параметры сервера.
type ServerConfig struct {
	// CredsPath — путь к файлу bcrypt-хэша (/opt/etc/sign-craze/admin.creds).
	CredsPath  string
	Status     StatusReader
	Config     ConfigRW
	Ports      PortsManager
	Excludes   ExcludesManager
	DPITargets DPITargetsManager

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

// Server — встроенный HTTP-сервер (порты 9090, 9091, опционально 9092 для routing UI).
type Server struct {
	clash *http.Server
	admin *http.Server
	// routingUI — третий сервер для UI-редактора routing. nil если RoutingUIEnabled=false.
	routingUI         *http.Server
	routingUIPort     uint16       // фактически выбранный порт (после FindFreePort)
	routingUIListener net.Listener // зарезервированный listener; передаётся в routingUI.Serve

	creds []byte
	cfg   ServerConfig
}

// NewServer создаёт Server и загружает (или генерирует) учётные данные.
func NewServer(cfg ServerConfig) (*Server, error) {
	creds, err := LoadOrCreateCreds(cfg.CredsPath)
	if err != nil {
		return nil, err
	}

	s := &Server{creds: creds, cfg: cfg}

	clashMux := http.NewServeMux()
	registerClashRoutes(clashMux, s)

	adminMux := http.NewServeMux()
	registerAdminRoutes(adminMux, s)

	s.clash = &http.Server{
		Addr: ":9090",
		Handler: recoverMiddleware(securityHeaders(s.basicAuth(
			perRouteWriteDeadline(clashMux),
		))),
		ReadTimeout:    15 * time.Second,
		WriteTimeout:   0, // 0 — для WebSocket; non-WS-роуты получают deadline через perRouteWriteDeadline
		IdleTimeout:    60 * time.Second,
		MaxHeaderBytes: 8 * 1024,
	}
	s.admin = &http.Server{
		Addr: ":9091",
		Handler: recoverMiddleware(securityHeaders(s.basicAuth(
			originGuard(adminMux),
		))),
		ReadTimeout:    15 * time.Second,
		WriteTimeout:   30 * time.Second,
		IdleTimeout:    60 * time.Second,
		MaxHeaderBytes: 8 * 1024,
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
			Handler: recoverMiddleware(securityHeaders(s.basicAuth(
				originGuard(routingMux),
			))),
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

// Start запускает все листенеры (clash + admin + опционально routing UI).
// Блокируется до отмены ctx или критической ошибки.
func (s *Server) Start(ctx context.Context) error {
	errCh := make(chan error, 3)

	go func() {
		slog.Info("web: clash API запущен", "addr", s.clash.Addr)
		if err := s.clash.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("clash listener: %w", err)
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
	if err := s.clash.Shutdown(shutCtx); err != nil {
		errs = append(errs, fmt.Errorf("clash: %w", err))
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

// basicAuth — middleware: требует Basic Auth (логин admin, пароль из admin.creds).
func (s *Server) basicAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, password, ok := r.BasicAuth()
		if !ok || user != adminUsername || CheckPassword(s.creds, password) != nil {
			w.Header().Set("WWW-Authenticate", `Basic realm="sign-craze"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// securityHeaders проставляет защитные заголовки на каждый ответ.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; script-src 'self'")
		next.ServeHTTP(w, r)
	})
}

// originGuard защищает state-changing запросы от CSRF: Origin должен совпадать с Host.
// Применяется только к POST/PUT/DELETE/PATCH; GET/HEAD пропускаются.
func originGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}
		origin := r.Header.Get("Origin")
		if origin == "" {
			// Нет Origin → потенциально curl/CLI, но в браузере он всегда выставляется.
			// Для admin API требуем явного Origin, чтобы исключить CSRF-формы.
			referer := r.Header.Get("Referer")
			if referer == "" {
				http.Error(w, "Forbidden: Origin required", http.StatusForbidden)
				return
			}
			origin = referer
		}
		if !sameHostOrigin(origin, r.Host) {
			http.Error(w, "Forbidden: cross-origin", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// sameHostOrigin проверяет, что origin указывает на тот же host, что и Host-header.
// Сравнение чувствительно к порту: http://localhost:9091 ↔ Host: localhost:9091.
func sameHostOrigin(origin, host string) bool {
	// Origin формата scheme://host[:port][/...].
	idx := strings.Index(origin, "://")
	if idx < 0 {
		return false
	}
	rest := origin[idx+3:]
	if slash := strings.Index(rest, "/"); slash >= 0 {
		rest = rest[:slash]
	}
	return rest == host
}

// perRouteWriteDeadline ставит WriteDeadline на ответ, если path не относится к WS.
// Защита от slowloris для Clash-сервера, у которого WriteTimeout=0 ради WebSocket.
func perRouteWriteDeadline(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, isWS := wsRoutes[r.URL.Path]; !isWS {
			rc := http.NewResponseController(w)
			if err := rc.SetWriteDeadline(time.Now().Add(nonWSWriteDeadline)); err != nil {
				slog.Debug("web: SetWriteDeadline", "err", err)
			}
		}
		next.ServeHTTP(w, r)
	})
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
