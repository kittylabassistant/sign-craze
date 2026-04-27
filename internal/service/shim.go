package service

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"text/template"

	"github.com/kittylabassistant/sign-craze/internal/atomicfs"
	"github.com/kittylabassistant/sign-craze/internal/log"
)

const (
	// DefaultShimPath — путь к init.d shim на роутере.
	DefaultShimPath = "/opt/etc/init.d/S05signcraze"
	// DefaultBinPath — путь к бинарю sign-craze на роутере.
	DefaultSignCrazeBin = "/opt/sbin/sign-craze"
)

// shimTemplate — шаблон init.d shim (~20 строк, генерируется sign-craze).
var shimTemplate = template.Must(template.New("shim").Parse(`#!/bin/sh
# Сгенерировано sign-craze. Не редактировать вручную.
SC={{ .BinPath }}
case "$1" in
    start)   exec "$SC" --service-start ;;
    stop)    exec "$SC" --service-stop ;;
    restart) exec "$SC" --service-restart ;;
    status)  exec "$SC" --service-status ;;
    *)       echo "Использование: $0 {start|stop|restart|status}"; exit 1 ;;
esac
`))

// ShimParams задаёт параметры для генерации init.d shim.
type ShimParams struct {
	BinPath string // путь к бинарю sign-craze; по умолчанию DefaultSignCrazeBin
}

// WriteShim генерирует init.d shim и атомарно записывает его в shimPath.
// Если файл уже существует и содержимое совпадает (по SHA256) — запись пропускается.
func WriteShim(shimPath string, p ShimParams) error {
	if p.BinPath == "" {
		p.BinPath = DefaultSignCrazeBin
	}

	var buf bytes.Buffer
	if err := shimTemplate.Execute(&buf, p); err != nil {
		return fmt.Errorf("service shim: рендеринг шаблона: %w", err)
	}
	content := buf.Bytes()

	// идемпотентность: пропускаем запись если содержимое не изменилось
	if existing, err := os.ReadFile(shimPath); err == nil {
		if sha256.Sum256(existing) == sha256.Sum256(content) {
			log.L().Debug("shim актуален, запись пропущена", "path", shimPath)
			return nil
		}
	}

	if err := atomicfs.WriteFileAtomic(shimPath, content, 0o755); err != nil {
		return fmt.Errorf("service shim: запись файла: %w", err)
	}

	log.L().Info("init.d shim записан", "path", shimPath)
	return nil
}

// RenderShim возвращает содержимое shim без записи на диск (используется в тестах).
func RenderShim(p ShimParams) ([]byte, error) {
	if p.BinPath == "" {
		p.BinPath = DefaultSignCrazeBin
	}
	var buf bytes.Buffer
	if err := shimTemplate.Execute(&buf, p); err != nil {
		return nil, fmt.Errorf("service shim: рендеринг: %w", err)
	}
	return buf.Bytes(), nil
}
