package errors

import "errors"

// Sentinel-ошибки, используемые во всех пакетах sign-craze.
// Сравнение через errors.Is.
var (
	// ErrNotInstalled — sing-box или sign-craze не установлен.
	ErrNotInstalled = errors.New("не установлен")

	// ErrAlreadyInstalled — попытка повторной установки без флага --reinstall.
	ErrAlreadyInstalled = errors.New("уже установлен")

	// ErrLockHeld — файловая блокировка удерживается другим процессом.
	ErrLockHeld = errors.New("блокировка занята")

	// ErrCoreDown — процесс sing-box не запущен или недоступен.
	ErrCoreDown = errors.New("sing-box не запущен")

	// ErrDPIDown — процесс nfqws2 не запущен или недоступен.
	ErrDPIDown = errors.New("nfqws2 не запущен")

	// ErrAlreadyRunning — сервис уже запущен, повторный старт запрещён.
	ErrAlreadyRunning = errors.New("сервис уже запущен")

	// ErrNotRunning — попытка остановить незапущенный сервис.
	ErrNotRunning = errors.New("сервис не запущен")

	// ErrConfigInvalid — конфигурационный файл не прошёл валидацию sing-box.
	ErrConfigInvalid = errors.New("конфиг невалиден")

	// ErrChecksumMismatch — SHA256 скачанного файла не совпадает с ожидаемым.
	ErrChecksumMismatch = errors.New("контрольная сумма не совпадает")

	// ErrNoSpace — недостаточно свободного места на /opt.
	ErrNoSpace = errors.New("недостаточно места на /opt")

	// ErrUnsupportedArch — архитектура не поддерживается.
	ErrUnsupportedArch = errors.New("архитектура не поддерживается")
)

// Wrap оборачивает err с контекстным сообщением.
// Эквивалент fmt.Errorf("%s: %w", msg, err) без импорта fmt.
func Wrap(msg string, err error) error {
	return &wrappedError{msg: msg, err: err}
}

type wrappedError struct {
	msg string
	err error
}

func (e *wrappedError) Error() string {
	return e.msg + ": " + e.err.Error()
}

func (e *wrappedError) Unwrap() error {
	return e.err
}

// Is реализует совместимость с errors.Is для sentinel-ошибок.
func Is(err, target error) bool {
	return errors.Is(err, target)
}

// Join объединяет несколько ошибок в одну (обёртка над errors.Join из Go 1.20+).
func Join(errs ...error) error {
	return errors.Join(errs...)
}
