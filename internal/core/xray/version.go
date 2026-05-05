package xray

import (
	"context"
	"fmt"
	"strings"

	"github.com/kittylabassistant/sign-craze/internal/exectx"
)

// BinaryVersion запрашивает версию установленного xray через `xray version`.
// Возвращает строку вида "1.8.4" без префикса "v".
func BinaryVersion(ctx context.Context, runner exectx.Runner, binPath string) (string, error) {
	res, err := runner.Run(ctx, binPath, "version")
	if err != nil {
		return "", fmt.Errorf("xray version: %w", err)
	}
	return parseVersion(string(res.Stdout))
}

// parseVersion извлекает версию из вывода `xray version`.
//
// Ожидаемый формат первой строки:
//
//	Xray 1.8.4 (Xray, Penetrates Everything.) Custom (go1.21.5 linux/amd64)
//
// Также допустимы более старые/будущие варианты:
//
//	Xray Core v1.8.4 ...
//	xray version 1.8.4
//
// Стратегия: ищем первый токен, начинающийся с цифры (или "v" + цифра).
// Префикс "v" стрипаем.
func parseVersion(output string) (string, error) {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		for _, p := range parts {
			p = strings.TrimPrefix(p, "v")
			if len(p) > 0 && p[0] >= '0' && p[0] <= '9' {
				// Отбрасываем хвост типа "(Xray," если приклеился.
				if idx := strings.IndexAny(p, "(,"); idx > 0 {
					p = p[:idx]
				}
				return p, nil
			}
		}
		// Первая непустая строка прочитана — версия в ней не нашлась.
		break
	}
	return "", fmt.Errorf("xray version: не удалось распознать версию из вывода: %q", output)
}
