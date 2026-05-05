package mihomo

import (
	"context"
	"fmt"
	"strings"

	"github.com/kittylabassistant/sign-craze/internal/exectx"
)

// BinaryVersion запрашивает версию установленного mihomo через `mihomo -v`.
// Возвращает строку вида "1.19.24" без префикса "v".
func BinaryVersion(ctx context.Context, runner exectx.Runner, binPath string) (string, error) {
	res, err := runner.Run(ctx, binPath, "-v")
	if err != nil {
		return "", fmt.Errorf("mihomo -v: %w", err)
	}
	return parseVersion(string(res.Stdout))
}

// parseVersion извлекает версию из вывода `mihomo -v`.
//
// Ожидаемый формат первой строки:
//
//	Mihomo Meta v1.19.24 darwin/arm64 with go1.22.5 Mon Aug 12 12:00:00 UTC 2024
//
// Также поддержим старые сборки Clash.Meta:
//
//	Clash Meta v1.18.0 ...
//
// Стратегия: ищем первый токен, начинающийся с "v" + цифра (или сразу с цифры),
// стрипаем "v".
func parseVersion(output string) (string, error) {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		for _, p := range parts {
			trimmed := strings.TrimPrefix(p, "v")
			if len(trimmed) == 0 || trimmed[0] < '0' || trimmed[0] > '9' {
				continue
			}
			// Должно быть похоже на семвер: содержит хотя бы одну точку.
			if !strings.Contains(trimmed, ".") {
				continue
			}
			// Отрезать хвост типа "(...)".
			if idx := strings.IndexAny(trimmed, "(,"); idx > 0 {
				trimmed = trimmed[:idx]
			}
			return trimmed, nil
		}
		break
	}
	return "", fmt.Errorf("mihomo -v: не удалось распознать версию из вывода: %q", output)
}
