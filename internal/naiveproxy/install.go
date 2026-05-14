package naiveproxy

import (
	"fmt"

	"github.com/kittylabassistant/sign-craze/internal/log"
)

// DefaultBinPath — путь установки бинаря naive.
const DefaultBinPath = "/opt/sbin/naive"

// Install устанавливает бинарь naive из tar.xz tarball.
// Алгоритм:
//  1. Стриминговая xz+tar распаковка через ExtractBinary.
//  2. Atomic replace с backup через atomicfs.BackupAndReplaceFromReader.
//  3. Права 0o755.
func Install(tarXZPath, binDst string) error {
	log.L().Info("установка naive", "tarball", tarXZPath, "dst", binDst)
	if err := ExtractBinary(tarXZPath, binDst, 0o755); err != nil {
		return fmt.Errorf("naiveproxy install: %w", err)
	}
	log.L().Info("naive установлен", "path", binDst)
	return nil
}
