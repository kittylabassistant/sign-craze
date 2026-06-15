package singbox

import (
	"testing"

	"github.com/kittylabassistant/sign-craze/pkg/types"
)

func TestAllocateNaiveListenPort_Default(t *testing.T) {
	if p := allocateNaiveListenPort(nil); p != defaultNaiveListenPort {
		t.Errorf("got %d, want %d", p, defaultNaiveListenPort)
	}
}

func TestAllocateNaiveListenPort_DefaultEmptyOutbounds(t *testing.T) {
	obs := []types.Outbound{
		{Tag: "direct", Type: "direct", Protocol: types.ProtocolDirect},
		{Tag: "vless-1", Type: "vless", Protocol: types.ProtocolVLESS},
	}
	if p := allocateNaiveListenPort(obs); p != defaultNaiveListenPort {
		t.Errorf("got %d, want %d", p, defaultNaiveListenPort)
	}
}

func TestAllocateNaiveListenPort_UsesExistingPort(t *testing.T) {
	obs := []types.Outbound{
		{Tag: "n1", Protocol: types.ProtocolNaive, Proto: &types.ProtoOpts{NaiveListenPort: 19000}},
	}
	if p := allocateNaiveListenPort(obs); p != 19000 {
		t.Errorf("got %d, want 19000", p)
	}
}
