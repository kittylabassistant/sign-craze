package cli

import (
	"bufio"
	"bytes"
	"strings"
	"testing"

	"github.com/kittylabassistant/sign-craze/internal/state"
	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// TestWizardURL_VLESS_PopulatesCanonical проверяет, что ParseCanonical применяется
// и canonical-поля (Protocol/TLS/Proto) заполняются в Outbound.
func TestWizardURL_VLESS_PopulatesCanonical(t *testing.T) {
	rawURL := "vless://test-uuid@vless.example.com:443" +
		"?security=reality&pbk=PUBLIC_KEY&sid=ab12&fp=chrome&sni=sni.example.com" +
		"#vless-tag"
	input := rawURL + "\n"

	r := bufio.NewReader(bytes.NewBufferString(input))
	out := &bytes.Buffer{}

	obs, err := wizardURL(r, out)
	if err != nil {
		t.Fatalf("wizardURL: %v", err)
	}
	if len(obs) != 1 {
		t.Fatalf("ожидался 1 outbound, получено %d", len(obs))
	}
	o := obs[0]

	if o.Protocol != types.ProtocolVLESS {
		t.Errorf("Protocol = %q, ожидалось %q", o.Protocol, types.ProtocolVLESS)
	}
	if o.TLS == nil {
		t.Fatal("TLS = nil, ожидалась Reality TLS")
	}
	if !o.TLS.Enabled {
		t.Error("TLS.Enabled должно быть true")
	}
	if o.TLS.Reality == nil {
		t.Fatal("TLS.Reality = nil")
	}
	if o.TLS.Reality.PublicKey != "PUBLIC_KEY" {
		t.Errorf("TLS.Reality.PublicKey = %q", o.TLS.Reality.PublicKey)
	}
	if o.Proto == nil {
		t.Fatal("Proto = nil")
	}
	if o.Proto.UUID != "test-uuid" {
		t.Errorf("Proto.UUID = %q, ожидалось test-uuid", o.Proto.UUID)
	}
	if o.Tag != "vless-tag" {
		t.Errorf("Tag = %q, ожидалось vless-tag", o.Tag)
	}
	if o.Server != "vless.example.com" {
		t.Errorf("Server = %q", o.Server)
	}
	if o.Port != 443 {
		t.Errorf("Port = %d, ожидалось 443", o.Port)
	}
}

// TestWizardURL_Socks5_Canonical проверяет, что socks5 URL обрабатывается
// через ParseCanonical. Legacy-парсер proxyparse.Parse удалён — другого пути
// разбора URL в проекте больше нет, поэтому Protocol обязан быть заполнен.
func TestWizardURL_Socks5_Canonical(t *testing.T) {
	rawURL := "socks5://alice:pass@10.0.0.1:1080"
	input := rawURL + "\n"

	r := bufio.NewReader(bytes.NewBufferString(input))
	out := &bytes.Buffer{}

	obs, err := wizardURL(r, out)
	if err != nil {
		t.Fatalf("wizardURL socks5: %v", err)
	}
	if len(obs) != 1 {
		t.Fatalf("ожидался 1 outbound, получено %d", len(obs))
	}
	o := obs[0]

	if o.Type != "socks" {
		t.Errorf("Type = %q, ожидалось socks", o.Type)
	}
	if o.Server != "10.0.0.1" {
		t.Errorf("Server = %q", o.Server)
	}
	if o.Port != 1080 {
		t.Errorf("Port = %d, ожидалось 1080", o.Port)
	}
	if o.Protocol != types.ProtocolSocks5 {
		t.Errorf("Protocol = %q, ожидалось %q", o.Protocol, types.ProtocolSocks5)
	}
	// Проверяем вывод: строка "Outbound:" должна присутствовать
	if !strings.Contains(out.String(), "Outbound:") {
		t.Errorf("ожидалась строка Outbound: в выводе, получено: %q", out.String())
	}
}

// TestWizardURL_UnknownSecurity_ReturnsError фиксирует ужесточение валидации
// после удаления legacy-парсера: раньше при ошибке ParseCanonical (неизвестный
// security=...) срабатывал fallback на proxyparse.Parse, который security
// молча игнорировал и возвращал валидный Outbound без TLS. Теперь fallback
// удалён — такой URL обязан вернуть понятную ошибку парсинга, а не тихий
// даунгрейд до "outbound без TLS".
func TestWizardURL_UnknownSecurity_ReturnsError(t *testing.T) {
	rawURL := "vless://test-uuid@vless.example.com:443?security=garbage"
	input := rawURL + "\n"

	r := bufio.NewReader(bytes.NewBufferString(input))
	out := &bytes.Buffer{}

	obs, err := wizardURL(r, out)
	if err == nil {
		t.Fatalf("ожидалась ошибка для security=garbage, получено obs=%+v", obs)
	}
	if !strings.Contains(err.Error(), "security") {
		t.Errorf("ошибка должна упоминать security, получено: %v", err)
	}
}

// TestParseProxyURLToOutbound_PbkWithoutReality_ReturnsError фиксирует ещё один
// случай ужесточения: pbk задан, но security != reality. Legacy-парсер такой
// pbk молча отбрасывал; ParseCanonical (единственный парсер теперь) возвращает
// ошибку.
func TestParseProxyURLToOutbound_PbkWithoutReality_ReturnsError(t *testing.T) {
	url := "vless://test-uuid@vless.example.com:443?security=tls&pbk=PUBLIC_KEY"
	_, _, err := parseProxyURLToOutbound(url)
	if err == nil {
		t.Fatal("ожидалась ошибка: pbk задан без security=reality")
	}
	if !strings.Contains(err.Error(), "pbk") {
		t.Errorf("ошибка должна упоминать pbk, получено: %v", err)
	}
}

// TestWizardURL_NaiveHTTPS_DefaultsListenPort — регрессия бага B3: интерактивный
// wizard должен проставлять NaiveListenPort=18443 по умолчанию точно так же,
// как это делает --proxy флаг (parseProxyURLToOutbound). До фикса wizardURL
// не применял naive-порт-дефолт → NaiveListenPort оставался 0, из-за чего
// singbox/render.go (renderCanonical, case ProtocolNaive) падал с ошибкой
// "NaiveListenPort не выделен" уже на PrepareAndValidate при --install.
func TestWizardURL_NaiveHTTPS_DefaultsListenPort(t *testing.T) {
	rawURL := "naive+https://user:pass@example.com:443"
	input := rawURL + "\n"

	r := bufio.NewReader(bytes.NewBufferString(input))
	out := &bytes.Buffer{}

	obs, err := wizardURL(r, out)
	if err != nil {
		t.Fatalf("wizardURL naive+https: %v", err)
	}
	if len(obs) != 1 {
		t.Fatalf("ожидался 1 outbound, получено %d", len(obs))
	}
	o := obs[0]

	if o.Protocol != types.ProtocolNaive {
		t.Fatalf("Protocol = %q, ожидалось %q", o.Protocol, types.ProtocolNaive)
	}
	if o.Proto == nil {
		t.Fatal("Proto = nil, ожидался заполненный NaiveListenPort")
	}
	if o.Proto.NaiveListenPort != 18443 {
		t.Errorf("NaiveListenPort = %d, ожидалось 18443 (default)", o.Proto.NaiveListenPort)
	}
}

// TestWizardURL_EmptyURL_ReturnsNil фиксирует существующий контракт: если
// пользователь ничего не ввёл (просто Enter), wizardURL возвращает (nil, nil)
// без ошибки — doInstall в этом случае откатывается на stub direct outbound.
func TestWizardURL_EmptyURL_ReturnsNil(t *testing.T) {
	r := bufio.NewReader(bytes.NewBufferString("\n"))
	out := &bytes.Buffer{}

	obs, err := wizardURL(r, out)
	if err != nil {
		t.Fatalf("wizardURL empty: %v", err)
	}
	if obs != nil {
		t.Errorf("obs = %v, ожидалось nil", obs)
	}
}

// TestDetectCoreFromProxyURL_PQVLESS — PQ-VLESS должен дать recommended=xray.
// Регрессия на случай если кто-то ослабит singbox/mihomo Validate без обновления тестов.
func TestDetectCoreFromProxyURL_PQVLESS(t *testing.T) {
	url := "vless://11111111-1111-1111-1111-111111111111@example.com:443" +
		"?encryption=mlkem768x25519plus&type=tcp&security=reality&pbk=AAA&sid=BB#pq"

	rec, all, ok := detectCoreFromProxyURL(url)
	if !ok {
		t.Fatal("detectCoreFromProxyURL ok=false, ожидалось true для валидного PQ-VLESS")
	}
	if rec != "xray" {
		t.Errorf("recommended=%q, ожидалось xray", rec)
	}
	if len(all) != 1 || all[0] != "xray" {
		t.Errorf("allCompatible=%v, ожидалось [xray]", all)
	}
}

// TestDetectCoreFromProxyURL_PlainVLESS — plain VLESS совместим со всеми, default=sing-box.
func TestDetectCoreFromProxyURL_PlainVLESS(t *testing.T) {
	url := "vless://11111111-1111-1111-1111-111111111111@example.com:443?encryption=none&type=tcp#test"

	rec, all, ok := detectCoreFromProxyURL(url)
	if !ok {
		t.Fatal("ok=false, ожидалось true")
	}
	if rec != "sing-box" {
		t.Errorf("recommended=%q, ожидалось sing-box (default при multi-match)", rec)
	}
	if len(all) != 3 {
		t.Errorf("allCompatible=%v, ожидалось 3 ядра", all)
	}
}

// TestDetectCoreFromProxyURL_InvalidURL — невалидный URL даёт ok=false.
func TestDetectCoreFromProxyURL_InvalidURL(t *testing.T) {
	rec, all, ok := detectCoreFromProxyURL("not-a-url")
	if ok {
		t.Errorf("ok=true для невалидного URL, ожидалось false")
	}
	if rec != "" || all != nil {
		t.Errorf("rec=%q all=%v, ожидалось пусто", rec, all)
	}
}

// TestParseProxyURLToOutbound_ReturnsRecommended — расширенная сигнатура
// должна возвращать recommended core вместе с Outbound.
func TestParseProxyURLToOutbound_ReturnsRecommended(t *testing.T) {
	url := "tuic://uuid:pass@example.com:443?congestion_control=bbr&udp_relay_mode=native#tuic"
	o, rec, err := parseProxyURLToOutbound(url)
	if err != nil {
		t.Fatalf("parseProxyURLToOutbound: %v", err)
	}
	if o.Protocol != types.ProtocolTUIC {
		t.Errorf("Protocol=%q, ожидалось %q", o.Protocol, types.ProtocolTUIC)
	}
	// TUIC: совместим sing-box+mihomo, sing-box default → recommended=sing-box.
	if rec != "sing-box" {
		t.Errorf("recommended=%q, ожидалось sing-box (default при наличии)", rec)
	}
}

// TestParseProxyURLToOutbound_NaiveHTTPS — naive+https URL должен дать
// Protocol=ProtocolNaive и NaiveListenPort=18443.
func TestParseProxyURLToOutbound_NaiveHTTPS(t *testing.T) {
	url := "naive+https://alice:secret@proxy.example.com:8443#naive-tag"
	o, _, err := parseProxyURLToOutbound(url)
	if err != nil {
		t.Fatalf("parseProxyURLToOutbound naive+https: %v", err)
	}
	if o.Protocol != types.ProtocolNaive {
		t.Errorf("Protocol=%q, ожидалось %q", o.Protocol, types.ProtocolNaive)
	}
	if o.Proto == nil {
		t.Fatal("Proto = nil, ожидались заполненные NaiveUsername/NaiveListenPort")
	}
	if o.Proto.NaiveListenPort != 18443 {
		t.Errorf("NaiveListenPort=%d, ожидалось 18443", o.Proto.NaiveListenPort)
	}
	if o.Proto.NaiveUsername != "alice" {
		t.Errorf("NaiveUsername=%q, ожидалось alice", o.Proto.NaiveUsername)
	}
	if o.Server != "proxy.example.com" {
		t.Errorf("Server=%q, ожидалось proxy.example.com", o.Server)
	}
	if o.Port != 8443 {
		t.Errorf("Port=%d, ожидалось 8443", o.Port)
	}
}

// TestParseInstallFlags — табличный тест общего парсера флагов install-хендлеров
// (--core/--proxy/--with-dpi/--with-naive/--preset/--inbound), вынесенного из
// четырёх дублировавших друг друга handleInstall/handleInstallAuto/
// handleInstallOffline/handleReinstall. Проверяет разбор всех флагов в
// пробельной и equals-форме, дефолт --inbound=state.DefaultInbound при
// отсутствии флага и поведение с неизвестным флагом/позиционным аргументом:
// parseStringFlag/parseBoolFlag не валидируют неизвестные токены, а оставляют
// их в rest в исходном порядке — parseInstallFlags этот контракт не меняет
// (rest нужен handleInstallOffline для позиционного пути к tarball).
func TestParseInstallFlags(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		want     installFlags
		wantRest []string
	}{
		{
			name: "все флаги, пробельная форма",
			args: []string{
				"--core", "xray",
				"--proxy", "vless://uuid@host:443",
				"--with-dpi",
				"--with-naive",
				"--preset", "ru-direct",
				"--inbound", "tun",
			},
			want: installFlags{
				core:      "xray",
				proxy:     "vless://uuid@host:443",
				withDPI:   true,
				withNaive: true,
				preset:    "ru-direct",
				inbound:   "tun",
			},
			wantRest: []string{},
		},
		{
			name: "все флаги, equals-форма",
			args: []string{
				"--core=mihomo",
				"--proxy=socks5://10.0.0.1:1080",
				"--with-dpi",
				"--with-naive",
				"--preset=block-ads",
				"--inbound=tproxy",
			},
			want: installFlags{
				core:      "mihomo",
				proxy:     "socks5://10.0.0.1:1080",
				withDPI:   true,
				withNaive: true,
				preset:    "block-ads",
				inbound:   "tproxy",
			},
			wantRest: []string{},
		},
		{
			name: "дефолты: ни один флаг не передан",
			args: []string{},
			want: installFlags{
				inbound: state.DefaultInbound,
			},
			wantRest: []string{},
		},
		{
			name: "неизвестный флаг и позиционный аргумент остаются в rest",
			args: []string{"/tmp/offline.tar.gz", "--unknown-flag", "--core", "sing-box"},
			want: installFlags{
				core:    "sing-box",
				inbound: state.DefaultInbound,
			},
			wantRest: []string{"/tmp/offline.tar.gz", "--unknown-flag"},
		},
		{
			name: "оборванный флаг без значения остаётся в rest",
			args: []string{"--core"},
			want: installFlags{
				inbound: state.DefaultInbound,
			},
			wantRest: []string{"--core"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, rest := parseInstallFlags(tc.args)
			if got != tc.want {
				t.Errorf("parseInstallFlags(%v) = %+v, want %+v", tc.args, got, tc.want)
			}
			if len(rest) != len(tc.wantRest) {
				t.Fatalf("rest = %v, want %v", rest, tc.wantRest)
			}
			for i := range rest {
				if rest[i] != tc.wantRest[i] {
					t.Errorf("rest[%d] = %q, want %q", i, rest[i], tc.wantRest[i])
				}
			}
		})
	}
}
