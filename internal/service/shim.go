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

// shimTemplate — шаблон init.d shim (~30 строк, генерируется sign-craze).
// set -eu + логирование stderr в boot.log: иначе init молча игнорирует
// падение --service-start, и юзер думает что сервис стартовал (safety-fixes #5).
// status не пишет в boot.log — он опрашивается часто и полезен в stderr.
var shimTemplate = template.Must(template.New("shim").Parse(`#!/bin/sh
# Сгенерировано sign-craze. Не редактировать вручную.
set -eu
SC={{ .BinPath }}
LOG=/opt/var/log/sign-craze/boot.log
mkdir -p "$(dirname "$LOG")" 2>/dev/null || true

run() {
    if ! "$SC" "$1" 2>>"$LOG"; then
        echo "sign-craze $1 завершился с ошибкой (см. $LOG)" >&2
        exit 1
    fi
}

case "${1:-}" in
    start)   run --service-start ;;
    stop)    run --service-stop ;;
    restart) run --service-restart ;;
    status)  exec "$SC" --service-status ;;
    *)       echo "Использование: $0 {start|stop|restart|status}" >&2; exit 1 ;;
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
