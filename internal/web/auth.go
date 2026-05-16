package web

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"

	"golang.org/x/crypto/bcrypt"
)

// LoadOrCreateCreds читает bcrypt-хэш из path.
// Если файл отсутствует — генерирует случайный пароль, выводит на stdout и сохраняет хэш.
func LoadOrCreateCreds(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err == nil {
		return bytes.TrimSpace(data), nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("web auth: чтение %s: %w", path, err)
	}

	raw := make([]byte, 18)
	if _, randErr := rand.Read(raw); randErr != nil {
		return nil, fmt.Errorf("web auth: генерация пароля: %w", randErr)
	}
	password := base64.RawURLEncoding.EncodeToString(raw)

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return nil, fmt.Errorf("web auth: bcrypt: %w", err)
	}

	if writeErr := os.WriteFile(path, append(hash, '\n'), 0o600); writeErr != nil {
		return nil, fmt.Errorf("web auth: сохранение %s: %w", path, writeErr)
	}

	fmt.Printf("sign-craze: пароль admin UI (показан один раз): %s\n", password)
	return hash, nil
}

// CheckPassword проверяет пароль против bcrypt-хэша.
func CheckPassword(hash []byte, password string) error {
	return bcrypt.CompareHashAndPassword(hash, []byte(password))
}
