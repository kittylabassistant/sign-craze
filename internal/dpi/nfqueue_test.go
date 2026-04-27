package dpi

import (
	"context"
	"testing"

	"github.com/kittylabassistant/sign-craze/internal/exectx"
)

func TestNFQueue_Enable_ДобавляетПравила(t *testing.T) {
	r := exectx.Mock(map[string]exectx.Result{
		// TCP — -C → нет → -A
		"iptables -t mangle -A signcraze_dpi -m mark ! --mark 0x53 -p tcp -j NFQUEUE --queue-num 200 --queue-bypass -m comment --comment signcraze:dpi-tcp": {ExitCode: 0},
		// UDP — -C → нет → -A
		"iptables -t mangle -A signcraze_dpi -m mark ! --mark 0x53 -p udp -j NFQUEUE --queue-num 200 --queue-bypass -m comment --comment signcraze:dpi-udp": {ExitCode: 0},
	})

	q := NewNFQueue(r)
	if err := q.Enable(context.Background(), 200, 0x53); err != nil {
		t.Fatalf("Enable: %v", err)
	}
}

func TestNFQueue_Enable_Идемпотентен(t *testing.T) {
	// -C возвращает 0 (правило уже есть) → -A не вызывается → нет ошибки
	r := exectx.Mock(map[string]exectx.Result{
		"iptables -t mangle -C signcraze_dpi -m mark ! --mark 0x53 -p tcp -j NFQUEUE --queue-num 200 --queue-bypass -m comment --comment signcraze:dpi-tcp": {ExitCode: 0},
		"iptables -t mangle -C signcraze_dpi -m mark ! --mark 0x53 -p udp -j NFQUEUE --queue-num 200 --queue-bypass -m comment --comment signcraze:dpi-udp": {ExitCode: 0},
	})

	q := NewNFQueue(r)
	if err := q.Enable(context.Background(), 200, 0x53); err != nil {
		t.Fatalf("Enable (повторный): %v", err)
	}
}

func TestNFQueue_Disable_УдаляетПравила(t *testing.T) {
	r := exectx.Mock(map[string]exectx.Result{
		// -C → 0 (правило есть) → -D
		"iptables -t mangle -C signcraze_dpi -m mark ! --mark 0x53 -p tcp -j NFQUEUE --queue-num 200 --queue-bypass -m comment --comment signcraze:dpi-tcp": {ExitCode: 0},
		"iptables -t mangle -D signcraze_dpi -m mark ! --mark 0x53 -p tcp -j NFQUEUE --queue-num 200 --queue-bypass -m comment --comment signcraze:dpi-tcp": {ExitCode: 0},
		"iptables -t mangle -C signcraze_dpi -m mark ! --mark 0x53 -p udp -j NFQUEUE --queue-num 200 --queue-bypass -m comment --comment signcraze:dpi-udp": {ExitCode: 0},
		"iptables -t mangle -D signcraze_dpi -m mark ! --mark 0x53 -p udp -j NFQUEUE --queue-num 200 --queue-bypass -m comment --comment signcraze:dpi-udp": {ExitCode: 0},
	})

	q := NewNFQueue(r)
	if err := q.Disable(context.Background(), 200, 0x53); err != nil {
		t.Fatalf("Disable: %v", err)
	}
}

func TestNFQueue_Disable_Идемпотентен(t *testing.T) {
	// -C → ошибка (правил нет) → DeleteRule возвращает nil (идемпотентно)
	r := exectx.Mock(map[string]exectx.Result{})

	q := NewNFQueue(r)
	if err := q.Disable(context.Background(), 200, 0x53); err != nil {
		t.Fatalf("Disable (правил нет): %v", err)
	}
}
