package web

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// ServerConfig содержит зависимости и параметры сервера.
type ServerConfig struct {
	// CredsPath — путь к файлу bcrypt-хэша (/opt/etc/sign-craze/admin.creds).
	CredsPath string
	Status    StatusReader
	Config    ConfigRW
	Ports     PortsManager
	Excludes  ExcludesManager
}

// Server — встроенный HTTP-сервер (порты 9090 и 9091).
type Server struct {
	clash *http.Server
	admin *http.Server
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
		Addr:         ":9090",
		Handler:      recoverMiddleware(s.basicAuth(clashMux)),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 0, // 0 — для WebSocket: без таймаута записи
		IdleTimeout:  60 * time.Second,
	}
	s.admin = &http.Server{
		Addr:         ":9091",
		Handler:      recoverMiddleware(s.basicAuth(adminMux)),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return s, nil
}

// Start запускает оба листенера. Блокируется до отмены ctx или критической ошибки.
func (s *Server) Start(ctx context.Context) error {
	errCh := make(chan error, 2)

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

// Stop выполняет graceful shutdown обоих серверов (таймаут 5 с).
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
	if len(errs) != 0 {
		return fmt.Errorf("web stop: %v", errs)
	}
	return nil
}

// basicAuth — middleware: требует Basic Auth (логин admin, пароль из admin.creds).
func (s *Server) basicAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, password, ok := r.BasicAuth()
		if !ok || CheckPassword(s.creds, password) != nil {
			w.Header().Set("WWW-Authenticate", `Basic realm="sign-craze"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
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
