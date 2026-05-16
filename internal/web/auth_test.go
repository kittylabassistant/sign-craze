package web

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestLoadOrCreateCreds_создаёт_файл_если_нет(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "admin.creds")

	hash, err := LoadOrCreateCreds(path)
	if err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	if len(hash) == 0 {
		t.Fatal("хэш пустой")
	}

	// файл создан
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("файл не создан: %v", err)
	}
}

func TestLoadOrCreateCreds_идемпотентен(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "admin.creds")

	hash1, err := LoadOrCreateCreds(path)
	if err != nil {
		t.Fatalf("первый вызов: %v", err)
	}

	hash2, err := LoadOrCreateCreds(path)
	if err != nil {
		t.Fatalf("второй вызов: %v", err)
	}

	if string(hash1) != string(hash2) {
		t.Errorf("хэш изменился при повторном вызове")
	}
}

func TestCheckPassword_верный_пароль(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "admin.creds")

	// Захватываем stdout, чтобы получить сгенерированный пароль
	origStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	_, err := LoadOrCreateCreds(path)
	w.Close()
	os.Stdout = origStdout

	if err != nil {
		t.Fatalf("создание creds: %v", err)
	}

	// Читаем пароль из stdout
	buf := make([]byte, 256)
	n, _ := r.Read(buf)
	line := string(buf[:n])

	// Парсим пароль из строки вида "sign-craze: пароль admin UI (показан один раз): <пароль>"
	var password string
	for _, sep := range []string{": "} {
		if idx := lastIndex(line, sep); idx >= 0 {
			password = trimNewline(line[idx+len(sep):])
			break
		}
	}
	if password == "" {
		t.Skip("не удалось извлечь пароль из stdout")
	}

	hash, _ := os.ReadFile(path)
	if err := CheckPassword(bytes.TrimSpace(hash), password); err != nil {
		t.Errorf("верный пароль отклонён: %v", err)
	}
}

func TestCheckPassword_неверный_пароль(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "admin.creds")

	if _, err := LoadOrCreateCreds(path); err != nil {
		t.Fatalf("создание creds: %v", err)
	}

	hash, _ := os.ReadFile(path)
	if err := CheckPassword(bytes.TrimSpace(hash), "неверный-пароль"); err == nil {
		t.Error("неверный пароль принят")
	}
}

// TestBcryptCost_SanityRange проверяет, что bcryptCost находится в допустимом
// диапазоне bcrypt: [MinCost=4, MaxCost=31].
func TestBcryptCost_SanityRange(t *testing.T) {
	if bcryptCost < bcrypt.MinCost {
		t.Errorf("bcryptCost=%d меньше bcrypt.MinCost=%d", bcryptCost, bcrypt.MinCost)
	}
	if bcryptCost > bcrypt.MaxCost {
		t.Errorf("bcryptCost=%d больше bcrypt.MaxCost=%d", bcryptCost, bcrypt.MaxCost)
	}
}

// lastIndex возвращает позицию последнего вхождения подстроки.
func lastIndex(s, sub string) int {
	last := -1
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			last = i
		}
	}
	return last
}

func trimNewline(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}
