package dpi

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	_ "embed"

	"github.com/kittylabassistant/sign-craze/internal/atomicfs"
	"github.com/kittylabassistant/sign-craze/internal/log"
)

//go:embed templates/nfqws2.conf.tmpl
var nfqws2ConfTmpl string

// DefaultBlobDir — каталог для blob-payload'ов nfqws2 (quic_initial.bin,
// tls_clienthello.bin и т.п.). Распаковывается из .ipk во время Install.
const DefaultBlobDir = "/opt/etc/sign-craze/blobs"

// DefaultQueueNum — номер NFQUEUE по умолчанию. 300 совпадает с upstream
// nfqws2-keenetic, чтобы избежать конфликта если оператор параллельно ставил
// nfqws2-keenetic вручную через Entware.
const DefaultQueueNum = 300

// ConfigParams задаёт параметры для генерации nfqws2.conf и сборки cmdline.
// Структура отражает раздельные блоки args в upstream nfqws2.conf v1.1.5.
type ConfigParams struct {
	// ISPInterface — исходящий интерфейс ISP (eth0, ppp0 и т.п.).
	// Определяется через `ip route show default`, первое слово после "dev".
	ISPInterface string
	// QueueNum — номер очереди NFQUEUE. По умолчанию DefaultQueueNum (300).
	QueueNum int
	// PolicyName — имя политики (по умолчанию "signcraze").
	PolicyName string
	// BlobDir — каталог с blob-файлами (--blob-dir в --lua-init).
	BlobDir string

	// BaseArgs — глобальные args (--lua-init, --blob-dir и т.п.).
	BaseArgs string
	// Args — TCP/TLS+HTTP стратегия (порты 443, 80, 1984, 5222).
	Args string
	// ArgsQUIC — QUIC-стратегия (UDP 443).
	ArgsQUIC string
	// ArgsUDP — прочий UDP (Discord voice, STUN, WireGuard, mTProto).
	ArgsUDP string
	// ExtraArgs — дополнительные args (--hostlist=, lists и т.п.).
	// Если HostlistPath задан, --hostlist=<path> добавляется автоматически.
	ExtraArgs string

	// HostlistPath — путь к файлу со списком доменов для selective DPI.
	// Если пуст — флаг --hostlist не добавляется (desync для всего трафика).
	HostlistPath string
}

// Дефолтные стратегии из github.com/nfqws/nfqws2-keenetic v1.1.5 (MIT).
// Покрывают YouTube (TLS+QUIC) и Discord (UDP voice/STUN + TLS).
const (
	defaultBaseArgs = "--lua-init --blob-dir=" + DefaultBlobDir

	defaultArgsQUIC = "--filter-udp=443 --filter-l7=quic " +
		"--payload=quic_initial " +
		"--lua-desync=fake:blob=quic_initial:repeats=11"

	defaultArgsUDP = "--filter-udp=590-600,1400,3478-3481,5349,19294-19344,49152-65535 " +
		"--filter-l7=wireguard,stun,discord,mtproto,unknown --out-range=<n2 " +
		"--payload=wireguard_initiation,wireguard_response,wireguard_cookie,stun,discord_ip_discovery,mtproto_initial,unknown " +
		"--lua-desync=circular:fails=2:time=300:retrans=3:nld=2 " +
		"--lua-desync=fake:repeats=6:strategy=1 " +
		"--lua-desync=fake:blob=quic_initial:repeats=6:strategy=2"

	defaultArgs = "--filter-tcp=443,80,1984,5222 --filter-l7=http,tls,mtproto " +
		"--payload=tls_client_hello,mtproto_initial " +
		"--lua-desync=circular:fails=2:time=300:retrans=3:nld=2 " +
		"--lua-desync=fake:blob=tls_clienthello:tls_mod=rnd,dupsid,sni=fonts.google.com:tcp_seq=10000:strategy=1 " +
		"--lua-desync=multisplit:pos=1,midsld:seqovl=1:seqovl_pattern=tls_clienthello:tcp_ts_up:strategy=1 " +
		"--lua-desync=fake:blob=0x00000000:tcp_ack=-66000:tls_mod=rnd,dupsid,sni=www.google.com:repeats=2:strategy=2 " +
		"--lua-desync=multisplit:pos=1,midsld:strategy=2 " +
		"--lua-desync=hostfakesplit:host=ozon.ru:midhost=host-2:seqovl=sniext+3:seqovl_pattern=tls_clienthello:badsum:tcp_md5:tcp_ts_up:strategy=3 " +
		"--lua-desync=hostfakesplit:tcp_md5:tcp_ts_up:strategy=3"
)

// DefaultConfigParams возвращает параметры по умолчанию (стратегии upstream).
func DefaultConfigParams() ConfigParams {
	return ConfigParams{
		ISPInterface: "eth0",
		QueueNum:     DefaultQueueNum,
		PolicyName:   "signcraze",
		BlobDir:      DefaultBlobDir,
		BaseArgs:     defaultBaseArgs,
		Args:         defaultArgs,
		ArgsQUIC:     defaultArgsQUIC,
		ArgsUDP:      defaultArgsUDP,
		ExtraArgs:    "",
	}
}

// BuildCmdline собирает командную строку nfqws2 по схеме upstream
// (etc/init.d/common):
//
//	nfqws2 --user=nobody --qnum=<N> <BaseArgs>
//	  <ArgsUDP> --new <ArgsQUIC> --new <Args> <ExtraArgs>
//
// Маркер "--new" отделяет независимые стратегии. Возвращает []string без
// первого аргумента (бинаря) — caller подставляет binPath сам.
func (p ConfigParams) BuildCmdline() []string {
	args := []string{
		"--user=nobody",
		fmt.Sprintf("--qnum=%d", p.QueueNum),
	}
	args = append(args, splitArgs(p.BaseArgs)...)

	if udp := splitArgs(p.ArgsUDP); len(udp) > 0 {
		args = append(args, udp...)
		args = append(args, "--new")
	}
	if quic := splitArgs(p.ArgsQUIC); len(quic) > 0 {
		args = append(args, quic...)
		args = append(args, "--new")
	}
	args = append(args, splitArgs(p.Args)...)

	extra := p.ExtraArgs
	if p.HostlistPath != "" {
		if extra != "" {
			extra += " "
		}
		extra += "--hostlist=" + p.HostlistPath
	}
	args = append(args, splitArgs(extra)...)
	return args
}

// splitArgs разбивает строку на токены по whitespace. Достаточно простой
// версии без учёта кавычек: дефолтные стратегии не содержат пробелов внутри
// одного токена (запятые/двоеточия не требуют экранирования).
func splitArgs(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Fields(s)
}

// WriteHostlist атомарно записывает список доменов в файл по одному на строку.
// Используется как источник для nfqws2 --hostlist=<path>.
func WriteHostlist(path string, targets []string) error {
	if path == "" {
		return fmt.Errorf("dpi hostlist: пустой path")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("dpi hostlist: создание директории: %w", err)
	}
	var buf bytes.Buffer
	for _, t := range targets {
		t = trimSpace(t)
		if t == "" {
			continue
		}
		buf.WriteString(t)
		buf.WriteByte('\n')
	}
	if err := atomicfs.WriteFileAtomic(path, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("dpi hostlist: запись %s: %w", path, err)
	}
	log.L().Info("dpi hostlist сгенерирован", "path", path, "entries", len(targets))
	return nil
}

func trimSpace(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\r' || s[start] == '\n') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\r' || s[end-1] == '\n') {
		end--
	}
	return s[start:end]
}

// GenerateConfig генерирует nfqws2.conf из шаблона и атомарно записывает в dstPath.
// Файл — диагностический артефакт для оператора. Реальный cmdline формируется
// через ConfigParams.BuildCmdline() в lifecycle.
func GenerateConfig(params ConfigParams, dstPath string) error {
	if params.ISPInterface == "" {
		return fmt.Errorf("dpi config: ISPInterface не задан")
	}
	if params.QueueNum <= 0 {
		return fmt.Errorf("dpi config: QueueNum должен быть > 0")
	}
	if params.PolicyName == "" {
		return fmt.Errorf("dpi config: PolicyName не задан")
	}

	tmpl, err := template.New("nfqws2.conf").Parse(nfqws2ConfTmpl)
	if err != nil {
		return fmt.Errorf("dpi config: парсинг шаблона: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, params); err != nil {
		return fmt.Errorf("dpi config: рендеринг шаблона: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
		return fmt.Errorf("dpi config: создание директории конфига: %w", err)
	}

	if err := atomicfs.WriteFileAtomic(dstPath, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("dpi config: запись %s: %w", dstPath, err)
	}

	log.L().Info("nfqws2.conf сгенерирован", "path", dstPath, "iface", params.ISPInterface)
	return nil
}

// DetectISPInterface определяет ISP-интерфейс через `ip route show default`.
// Парсит первое вхождение "dev <iface>" в выводе.
func DetectISPInterface(routeOutput string) (string, error) {
	for _, line := range splitLines(routeOutput) {
		fields := splitFields(line)
		for i, f := range fields {
			if f == "dev" && i+1 < len(fields) {
				return fields[i+1], nil
			}
		}
	}
	return "", fmt.Errorf("dpi config: не удалось определить ISP-интерфейс из вывода ip route")
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i, c := range s {
		if c == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func splitFields(s string) []string {
	var fields []string
	inField := false
	start := 0
	for i, c := range s {
		if c == ' ' || c == '\t' {
			if inField {
				fields = append(fields, s[start:i])
				inField = false
			}
		} else {
			if !inField {
				start = i
				inField = true
			}
		}
	}
	if inField {
		fields = append(fields, s[start:])
	}
	return fields
}
