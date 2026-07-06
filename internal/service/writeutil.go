package service

import (
	"crypto/sha256"
	"fmt"
	"os"

	"github.com/kittylabassistant/sign-craze/internal/atomicfs"
	"github.com/kittylabassistant/sign-craze/internal/log"
)

// writeIfChanged атомарно записывает content в path, но только если файл
// отсутствует или его текущее содержимое отличается от content (по SHA256).
// Общий хелпер для WriteShim (init.d shim) и WriteHook (netfilter.d hook):
// обе функции генерируют небольшой shell-скрипт и должны избегать лишней
// записи на flash/USB роутера при каждом вызове (--reapply, повторный
// --install и т.п.) — сравнение по хешу и есть идемпотентность.
//
// changed=true означает, что файл был создан или перезаписан. perm
// применяется к финальному файлу (через atomicfs.WriteFileAtomic). label
// используется в логах и в тексте ошибки записи (например "init.d shim",
// "netfilter.d hook").
func writeIfChanged(path string, content []byte, perm os.FileMode, label string) (changed bool, err error) {
	if existing, readErr := os.ReadFile(path); readErr == nil {
		if sha256.Sum256(existing) == sha256.Sum256(content) {
			log.L().Debug(label+" актуален, запись пропущена", "path", path)
			return false, nil
		}
	}

	if writeErr := atomicfs.WriteFileAtomic(path, content, perm); writeErr != nil {
		return false, fmt.Errorf("%s: запись файла: %w", label, writeErr)
	}

	log.L().Info(label+" записан", "path", path)
	return true, nil
}
