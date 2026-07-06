package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kittylabassistant/sign-craze/internal/exectx"
)

// TestDeadlineCtx_DetachedFromParentCancel — отмена родительского ctx
// (например Ctrl+C пользователя) не должна прерывать проверку конфига,
// уже стартовавшую под DeadlineCtx.
func TestDeadlineCtx_DetachedFromParentCancel(t *testing.T) {
	parent, parentCancel := context.WithCancel(context.Background())
	cctx, cancel := DeadlineCtx(parent, time.Minute)
	defer cancel()

	parentCancel()

	select {
	case <-cctx.Done():
		t.Fatal("cctx завершился из-за отмены parent — DeadlineCtx должен быть detached (context.WithoutCancel)")
	default:
	}
}

// TestDeadlineCtx_EnforcesTimeout — cctx обязан истечь по DeadlineExceeded
// после timeout, даже если родитель никогда не отменяется.
func TestDeadlineCtx_EnforcesTimeout(t *testing.T) {
	cctx, cancel := DeadlineCtx(context.Background(), 10*time.Millisecond)
	defer cancel()

	select {
	case <-cctx.Done():
		if !errors.Is(cctx.Err(), context.DeadlineExceeded) {
			t.Errorf("cctx.Err() = %v, ожидалось context.DeadlineExceeded", cctx.Err())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cctx не завершился по таймауту за разумное время")
	}
}

// TestCheckConfigError покрывает оба формата ошибки, общих для
// xray/mihomo/sing-box: таймаут (cctx истёк) и generic (любая другая
// ошибка выполнения) — включая разные label ядер и краевые случаи с
// пустыми stderr/stdout.
func TestCheckConfigError(t *testing.T) {
	expiredCtx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	<-expiredCtx.Done() // гарантированно истёк до вызова CheckConfigError

	tests := []struct {
		name    string
		cctx    context.Context
		label   string
		timeout time.Duration
		dur     time.Duration
		res     exectx.Result
		err     error
		want    string
	}{
		{
			name:    "таймаут xray",
			cctx:    expiredCtx,
			label:   "xray test",
			timeout: 180 * time.Second,
			dur:     3 * time.Second,
			res:     exectx.Result{ExitCode: -1, Stderr: []byte("some stderr"), Stdout: []byte("some stdout")},
			err:     errors.New("exec: context deadline exceeded"),
			want:    "xray test: таймаут 3m0s — медленный CPU или зависание (stderr: some stderr, stdout: some stdout)",
		},
		{
			name:    "generic-ошибка mihomo",
			cctx:    context.Background(),
			label:   "mihomo -t",
			timeout: 180 * time.Second,
			dur:     42 * time.Millisecond,
			res:     exectx.Result{ExitCode: 1, Stderr: []byte("bad config"), Stdout: []byte("")},
			err:     errors.New("exit status 1"),
			want:    "mihomo -t: exit status 1 (длительность: 42ms, exit: 1, stderr: bad config, stdout: )",
		},
		{
			name:    "generic-ошибка sing-box, пустой stderr",
			cctx:    context.Background(),
			label:   "sing-box check",
			timeout: 180 * time.Second,
			dur:     1500 * time.Millisecond,
			res:     exectx.Result{ExitCode: 2, Stderr: []byte(""), Stdout: []byte("parsed 3 rules")},
			err:     errors.New("invalid config"),
			want:    "sing-box check: invalid config (длительность: 1.5s, exit: 2, stderr: , stdout: parsed 3 rules)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CheckConfigError(tt.cctx, tt.label, tt.timeout, tt.dur, tt.res, tt.err)
			if got.Error() != tt.want {
				t.Errorf("CheckConfigError() = %q, ожидалось %q", got.Error(), tt.want)
			}
		})
	}
}

// TestCheckConfigError_WrapsUnderlyingErr — generic-ветка обязана оборачивать
// исходную ошибку через %w, чтобы errors.Is/errors.As работали у вызывающего.
func TestCheckConfigError_WrapsUnderlyingErr(t *testing.T) {
	underlying := errors.New("exit status 1")
	err := CheckConfigError(context.Background(), "xray test", CheckConfigTimeout, time.Second, exectx.Result{ExitCode: 1}, underlying)
	if !errors.Is(err, underlying) {
		t.Error("ожидался %w-wrap исходной ошибки (errors.Is должен находить underlying)")
	}
}
