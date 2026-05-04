package web

import (
	"context"
	"errors"
	"fmt"
	"net"
)

// ErrNoFreePort возвращается когда не нашлось свободного порта в диапазоне.
var ErrNoFreePort = errors.New("web: нет свободного порта в диапазоне")

// FindFreePort пытается слушать port, port+1, ... в течение maxAttempts попыток.
// Возвращает фактически выбранный порт и открытый listener (caller передаёт
// его в http.Server.Serve и не должен закрывать самостоятельно — Shutdown сам закроет).
//
// Если port=0 — kernel выбирает эфемерный порт; фактический порт читается из
// listener.Addr() и возвращается как первый параметр.
func FindFreePort(bind string, port uint16, maxAttempts int) (uint16, net.Listener, error) {
	if maxAttempts <= 0 {
		maxAttempts = 1
	}
	if bind == "" {
		bind = "0.0.0.0"
	}
	var cfg net.ListenConfig
	var lastErr error
	for i := 0; i < maxAttempts; i++ {
		candidate := port + uint16(i)
		addr := fmt.Sprintf("%s:%d", bind, candidate)
		l, err := cfg.Listen(context.Background(), "tcp", addr)
		if err == nil {
			actual := candidate
			if candidate == 0 {
				if tcpAddr, ok := l.Addr().(*net.TCPAddr); ok {
					actual = uint16(tcpAddr.Port)
				}
			}
			return actual, l, nil
		}
		lastErr = err
	}
	return 0, nil, fmt.Errorf("%w (от %d, попыток %d): %w", ErrNoFreePort, port, maxAttempts, lastErr)
}
