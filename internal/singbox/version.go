package singbox

import (
	"context"
	"fmt"
	"strings"

	"github.com/kittylabassistant/sign-craze/internal/exectx"
)

// BinaryVersion запрашивает версию установленного sing-box через `sing-box version`.
// Возвращает строку вида "1.10.0" без префикса "v".
func BinaryVersion(ctx context.Context, runner exectx.Runner, binPath string) (string, error) {
	res, err := runner.Run(ctx, binPath, "version")
	if err != nil {
		return "", fmt.Errorf("singbox version: %w", err)
	}
	return parseVersion(string(res.Stdout))
}

// parseVersion извлекает версию из вывода `sing-box version`.
// Ожидаемый формат первой строки: "sing-box version 1.10.0" или "sing-box 1.10.0".
func parseVersion(output string) (string, error) {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// ищем токен после "version" или берём последний токен первой строки
		parts := strings.Fields(line)
		for i, p := range parts {
			if strings.ToLower(p) == "version" && i+1 < len(parts) {
				return strings.TrimPrefix(parts[i+1], "v"), nil
			}
		}
		// запасной вариант: последнее слово первой строки
		if len(parts) > 0 {
			return strings.TrimPrefix(parts[len(parts)-1], "v"), nil
		}
	}
	return "", fmt.Errorf("singbox version: не удалось распознать версию из вывода: %q", output)
}
