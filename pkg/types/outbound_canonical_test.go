package types

import (
	"encoding/json"
	"testing"
)

func TestCanonical_IsSet(t *testing.T) {
	if (Canonical{}).IsSet() {
		t.Error("пустой Canonical должен быть IsSet=false")
	}
	if !(Canonical{Protocol: ProtocolVLESS}).IsSet() {
		t.Error("Canonical с Protocol должен быть IsSet=true")
	}
}

// TestCanonical_RoundTrip — все поля корректно сериализуются и десериализуются
// через json.Marshal/Unmarshal без потерь.
func TestCanonical_RoundTrip(t *testing.T) {
	want := Canonical{
		Protocol: ProtocolVLESS,
		Transport: &Transport{
			Kind: TransportXHTTP,
			Path: "/v2",
			Host: "example.com",
			Mode: XHTTPPacketUp,
		},
		TLS: &TLSConfig{
			Enabled:    true,
			ServerName: "example.com",
			ALPN:       []string{"h2", "http/1.1"},
			UTLS:       &UTLSConfig{Enabled: true, Fingerprint: "chrome"},
			Reality: &RealityConfig{
				Enabled:       true,
				PublicKey:     "abcdef",
				ShortID:       "deadbeef",
				MLDSA65Verify: "verify-key",
			},
		},
		Proto: &ProtoOpts{
			UUID:       "uuid-1",
			Flow:       "xtls-rprx-vision-udp443",
			Encryption: "mlkem768x25519plus",
		},
	}
	data, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got Canonical
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.Protocol != want.Protocol {
		t.Errorf("Protocol: got %q want %q", got.Protocol, want.Protocol)
	}
	if got.Transport.Kind != TransportXHTTP {
		t.Errorf("Transport.Kind: got %q want %q", got.Transport.Kind, TransportXHTTP)
	}
	if got.Transport.Mode != XHTTPPacketUp {
		t.Errorf("Transport.Mode: got %q want %q", got.Transport.Mode, XHTTPPacketUp)
	}
	if got.TLS.Reality == nil || got.TLS.Reality.MLDSA65Verify != "verify-key" {
		t.Errorf("TLS.Reality.MLDSA65Verify lost: %+v", got.TLS.Reality)
	}
	if got.Proto == nil || got.Proto.Flow != want.Proto.Flow {
		t.Errorf("Proto.Flow: got %+v want %q", got.Proto, want.Proto.Flow)
	}
}

// TestCanonical_OmitemptyEmptyJSON — пустой Canonical сериализуется в "{}",
// чтобы state.json не разбухал нулевыми полями для legacy-Outbound.
func TestCanonical_OmitemptyEmptyJSON(t *testing.T) {
	data, err := json.Marshal(Canonical{})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if string(data) != "{}" {
		t.Errorf("got %s, want {}", data)
	}
}
