package core

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/kittylabassistant/sign-craze/internal/exectx"
)

// CheckConfigTimeout — потолок длительности встроенной валидации конфига
// ядра (`sing-box check` / `xray test` / `mihomo -t`). На MIPS softfloat
// проверка большого конфига может занимать десятки секунд; SIGINT
// пользователя не должен прерывать её на середине (см. DeadlineCtx).
const CheckConfigTimeout = 180 * time.Second

// DeadlineCtx детачит ctx от родительской отмены (Ctrl+C пользователя не
// должен прервать уже начатую проверку конфига — иначе install/restart
// падает с непонятным "context canceled") и ограничивает выполнение
// таймаутом timeout. Caller обязан вызвать возвращённый cancel.
//
// Общий для всех адаптеров ядер шаг перед запуском `check`/`test`/`-t`.
func DeadlineCtx(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), timeout)
}

// CheckConfigError форматирует ошибку встроенной валидации конфига ядра в
// едином для sing-box/xray/mihomo стиле:
//   - если cctx (контекст, под которым выполнялась команда) истёк по
//     таймауту — отдельный текст "<label>: таймаут ... — медленный CPU или
//     зависание";
//   - иначе — generic-обёртка с длительностью выполнения, exit-кодом,
//     stderr и stdout.
//
// label — префикс сообщения ("xray test", "mihomo -t", "sing-box check");
// timeout — значение, использованное в DeadlineCtx (для текста
// таймаут-ошибки); dur — фактическая длительность выполнения команды;
// res/err — результат exectx.Runner.Run.
func CheckConfigError(cctx context.Context, label string, timeout, dur time.Duration, res exectx.Result, err error) error {
	if errors.Is(cctx.Err(), context.DeadlineExceeded) {
		return fmt.Errorf("%s: таймаут %s — медленный CPU или зависание (stderr: %s, stdout: %s)",
			label, timeout, res.Stderr, res.Stdout)
	}
	return fmt.Errorf("%s: %w (длительность: %s, exit: %d, stderr: %s, stdout: %s)",
		label, err, dur, res.ExitCode, res.Stderr, res.Stdout)
}
