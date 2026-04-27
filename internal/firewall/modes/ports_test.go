package modes

import (
	"strings"
	"testing"
)

func TestPortRules_Empty(t *testing.T) {
	if got := PortRules(nil, 0x53); got != nil {
		t.Errorf("ожидался nil для пустого списка, получено %+v", got)
	}
}

func TestPortRules_OneBatch(t *testing.T) {
	rules := PortRules([]uint16{443, 80}, 0x53)
	// Ожидаем: 1 TCP + 1 UDP правило в signcraze_ports + переход в PREROUTING.
	if len(rules) != 3 {
		t.Fatalf("len = %d, ожидалось 3 (tcp, udp, prerouting)", len(rules))
	}
	tcp := rules[0]
	if tcp.Chain != "signcraze_ports" {
		t.Errorf("Chain = %q, ожидалось signcraze_ports", tcp.Chain)
	}
	args := strings.Join(tcp.Args, " ")
	if !strings.Contains(args, "80,443") {
		t.Errorf("ожидался отсортированный список 80,443, получено: %s", args)
	}
	if !strings.Contains(args, "--set-mark 0x53") {
		t.Errorf("отсутствует MARK 0x53: %s", args)
	}

	prerouting := rules[2]
	if prerouting.Chain != "PREROUTING" {
		t.Errorf("последнее правило должно быть в PREROUTING, получено %q", prerouting.Chain)
	}
}

func TestPortRules_Batching(t *testing.T) {
	// 30 портов → 2 батча по 15 → 4 правила (TCP+UDP × 2) + 1 PREROUTING = 5.
	ports := make([]uint16, 30)
	for i := range ports {
		ports[i] = uint16(i + 1)
	}
	rules := PortRules(ports, 0x53)
	if len(rules) != 5 {
		t.Errorf("len = %d, ожидалось 5", len(rules))
	}
}

func TestChunk(t *testing.T) {
	got := chunk([]uint16{1, 2, 3, 4, 5}, 2)
	if len(got) != 3 {
		t.Errorf("ожидалось 3 батча, получено %d", len(got))
	}
	if len(got[2]) != 1 || got[2][0] != 5 {
		t.Errorf("последний батч = %v, ожидалось [5]", got[2])
	}
}
