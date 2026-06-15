package atomicfs

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// WriteFileAtomic записывает данные в dst атомарно через write→fsync→rename.
// Гарантирует, что читатели никогда не увидят частично записанный файл.
// perm применяется к финальному файлу.
func WriteFileAtomic(dst string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(dst)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("atomicfs: mkdir %s: %w", dir, err)
	}

	tmp, err := os.CreateTemp(dir, ".atomic-*")
	if err != nil {
		return fmt.Errorf("atomicfs: создание temp-файла: %w", err)
	}
	tmpName := tmp.Name()

	// удаляем temp-файл при любой ошибке до rename
	success := false
	defer func() {
		if !success {
			_ = os.Remove(tmpName)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("atomicfs: запись в temp-файл: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("atomicfs: fsync temp-файла: %w", err)
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("atomicfs: chmod temp-файла: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("atomicfs: закрытие temp-файла: %w", err)
	}

	if err := os.Rename(tmpName, dst); err != nil {
		return fmt.Errorf("atomicfs: rename %s → %s: %w", tmpName, dst, err)
	}

	syncDir(dir)

	success = true
	return nil
}

// WriteFileAtomicFromReader пишет содержимое r в dst через temp-файл с
// io.Copy (потоково, без буферизации всего содержимого в RAM).
// perm применяется к финальному файлу. Защищает от пика памяти при
// загрузке больших файлов на роутере с 128MB RAM (safety-fixes #14).
func WriteFileAtomicFromReader(dst string, r io.Reader, perm os.FileMode) error {
	dir := filepath.Dir(dst)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("atomicfs: mkdir %s: %w", dir, err)
	}

	tmp, err := os.CreateTemp(dir, ".atomic-*")
	if err != nil {
		return fmt.Errorf("atomicfs: создание temp-файла: %w", err)
	}
	tmpName := tmp.Name()

	success := false
	defer func() {
		if !success {
			_ = os.Remove(tmpName)
		}
	}()

	// 8KB буфера достаточно для streaming на 128MB роутере: больший размер
	// провоцирует heap-allocation и GC pressure при параллельных --update-geo.
	buf := make([]byte, 8*1024)
	if _, err := io.CopyBuffer(tmp, r, buf); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("atomicfs: копирование в temp: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("atomicfs: fsync temp: %w", err)
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("atomicfs: chmod temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("atomicfs: закрытие temp: %w", err)
	}
	if err := os.Rename(tmpName, dst); err != nil {
		return fmt.Errorf("atomicfs: rename %s → %s: %w", tmpName, dst, err)
	}
	syncDir(dir)
	success = true
	return nil
}

// syncDir делает fsync родительской директории — без этого rename
// может потеряться при power-off на ext3 data=writeback или VFAT
// (типичные FS на USB-флешке Keenetic). Ошибки игнорируются: VFAT
// возвращает EINVAL на dir Sync, и fsync не критичен для корректности
// чтения, только для durability — на чтение rename виден сразу.
func syncDir(dir string) {
	d, err := os.Open(dir)
	if err != nil {
		return
	}
	if syncErr := d.Sync(); syncErr != nil {
		// VFAT/некоторые FS возвращают EINVAL на dir Sync — это ожидаемо;
		// игнорируем без логирования (вызывается на каждый rename).
		_ = syncErr
	}
	_ = d.Close()
}

// BackupAndReplaceFromReader — потоковая версия BackupAndReplace.
// Бэкап существующего dst делается через rename (без чтения файла в RAM),
// новое содержимое стримится из r через WriteFileAtomicFromReader.
// Защита от OOM при установке больших бинарей на роутере с 128MB.
func BackupAndReplaceFromReader(dst string, r io.Reader, perm os.FileMode) (backupPath string, err error) {
	if _, statErr := os.Stat(dst); statErr == nil {
		backupPath = fmt.Sprintf("%s.bak.%d", dst, time.Now().Unix())
		if renameErr := os.Rename(dst, backupPath); renameErr != nil {
			return "", fmt.Errorf("atomicfs: rename для бэкапа: %w", renameErr)
		}
	}

	if err := WriteFileAtomicFromReader(dst, r, perm); err != nil {
		return backupPath, err
	}
	return backupPath, nil
}

// RestoreBackup восстанавливает dst из резервной копии backupPath и удаляет резервный файл.
func RestoreBackup(backupPath, dst string) error {
	data, err := os.ReadFile(backupPath)
	if err != nil {
		return fmt.Errorf("atomicfs: чтение резервной копии %s: %w", backupPath, err)
	}

	info, err := os.Stat(backupPath)
	if err != nil {
		return fmt.Errorf("atomicfs: stat резервной копии: %w", err)
	}

	if err := WriteFileAtomic(dst, data, info.Mode()); err != nil {
		return fmt.Errorf("atomicfs: восстановление из бэкапа: %w", err)
	}

	_ = os.Remove(backupPath)
	return nil
}
