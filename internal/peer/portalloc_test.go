package peer

import (
	"net"
	"strconv"
	"testing"

	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// TestAllocateMieruPort_Idempotent — повторный вызов не меняет уже выделенный
// порт.
func TestAllocateMieruPort_Idempotent(t *testing.T) {
	obs := []types.Outbound{
		{
			Tag:      "m1",
			Protocol: types.ProtocolMieru,
			Proto:    &types.ProtoOpts{MieruLocalPort: 42345},
		},
	}
	port, err := AllocateMieruPort(obs, 0)
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	if port != 42345 {
		t.Errorf("port = %d, ожидалось 42345 (idempotent)", port)
	}
	if obs[0].Proto.MieruLocalPort != 42345 {
		t.Errorf("Outbound.Proto.MieruLocalPort = %d (не изменилось)", obs[0].Proto.MieruLocalPort)
	}
}

// TestAllocateMieruPort_FreshAllocate — fresh outbound получает порт в
// допустимом диапазоне.
func TestAllocateMieruPort_FreshAllocate(t *testing.T) {
	obs := []types.Outbound{
		{Tag: "m1", Protocol: types.ProtocolMieru, Proto: &types.ProtoOpts{}},
	}
	port, err := AllocateMieruPort(obs, 0)
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	if port < MieruLocalPortBase || port > MieruLocalPortMax {
		t.Errorf("port = %d, ожидалось %d..%d", port, MieruLocalPortBase, MieruLocalPortMax)
	}
	if obs[0].Proto.MieruLocalPort != int(port) {
		t.Errorf("Outbound не обновлён: MieruLocalPort = %d", obs[0].Proto.MieruLocalPort)
	}
}

// TestAllocateMieruPort_AvoidCollision — два mieru-outbound'а получают разные
// порты.
func TestAllocateMieruPort_AvoidCollision(t *testing.T) {
	obs := []types.Outbound{
		{Tag: "m1", Protocol: types.ProtocolMieru, Proto: &types.ProtoOpts{}},
		{Tag: "m2", Protocol: types.ProtocolMieru, Proto: &types.ProtoOpts{}},
	}
	p1, err := AllocateMieruPort(obs, 0)
	if err != nil {
		t.Fatalf("Allocate m1: %v", err)
	}
	p2, err := AllocateMieruPort(obs, 1)
	if err != nil {
		t.Fatalf("Allocate m2: %v", err)
	}
	if p1 == p2 {
		t.Errorf("m1 и m2 получили одинаковый порт %d", p1)
	}
}

// TestAllocateMieruPort_SkipBusy — если базовый порт занят сторонним процессом,
// allocator пробует следующий.
func TestAllocateMieruPort_SkipBusy(t *testing.T) {
	// Занимаем 40000 чтобы Allocate пропустил.
	l, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(MieruLocalPortBase)))
	if err != nil {
		t.Skipf("базовый порт %d уже занят: %v (пропуск, не критично)", MieruLocalPortBase, err)
	}
	defer l.Close()

	obs := []types.Outbound{
		{Tag: "m1", Protocol: types.ProtocolMieru, Proto: &types.ProtoOpts{}},
	}
	port, err := AllocateMieruPort(obs, 0)
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	if int(port) == MieruLocalPortBase {
		t.Errorf("Allocate выдал занятый порт %d", port)
	}
}

// TestAllocateMieruPort_RejectsNonMieru — outbound не mieru → ошибка.
func TestAllocateMieruPort_RejectsNonMieru(t *testing.T) {
	obs := []types.Outbound{
		{Tag: "vless1", Protocol: types.ProtocolVLESS, Proto: &types.ProtoOpts{}},
	}
	_, err := AllocateMieruPort(obs, 0)
	if err == nil {
		t.Fatal("ожидалась ошибка для non-mieru")
	}
}

// TestAllocateMieruPort_OutOfBounds — индекс выходит за границы.
func TestAllocateMieruPort_OutOfBounds(t *testing.T) {
	obs := []types.Outbound{}
	_, err := AllocateMieruPort(obs, 0)
	if err == nil {
		t.Fatal("ожидалась ошибка для пустого slice")
	}
}

// TestAllocateAllMieruPorts_SkipsNonMieru — функция выделяет порты только
// для mieru-outbound'ов, не трогая остальные.
func TestAllocateAllMieruPorts_SkipsNonMieru(t *testing.T) {
	obs := []types.Outbound{
		{Tag: "vless1", Protocol: types.ProtocolVLESS, Proto: &types.ProtoOpts{}},
		{Tag: "m1", Protocol: types.ProtocolMieru, Proto: &types.ProtoOpts{}},
		{Tag: "trojan1", Protocol: types.ProtocolTrojan, Proto: &types.ProtoOpts{}},
		{Tag: "m2", Protocol: types.ProtocolMieru, Proto: &types.ProtoOpts{}},
	}
	if err := AllocateAllMieruPorts(obs); err != nil {
		t.Fatalf("AllocateAll: %v", err)
	}
	if obs[0].Proto.MieruLocalPort != 0 {
		t.Errorf("vless получил MieruLocalPort %d", obs[0].Proto.MieruLocalPort)
	}
	if obs[1].Proto.MieruLocalPort == 0 {
		t.Errorf("m1 не получил MieruLocalPort")
	}
	if obs[2].Proto.MieruLocalPort != 0 {
		t.Errorf("trojan получил MieruLocalPort")
	}
	if obs[3].Proto.MieruLocalPort == 0 {
		t.Errorf("m2 не получил MieruLocalPort")
	}
	if obs[1].Proto.MieruLocalPort == obs[3].Proto.MieruLocalPort {
		t.Errorf("m1 и m2 получили одинаковый порт")
	}
}
