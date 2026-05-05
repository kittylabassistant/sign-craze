package dpi

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"text/template"

	_ "embed"

	"github.com/kittylabassistant/sign-craze/internal/atomicfs"
	"github.com/kittylabassistant/sign-craze/internal/log"
)

//go:embed templates/nfqws2.conf.tmpl
var nfqws2ConfTmpl string

// ConfigParams задаёт параметры для генерации nfqws2.conf.
type ConfigParams struct {
	// ISPInterface — исходящий интерфейс ISP (eth0, ppp0 и т.п.).
	// Определяется через `ip route show default`, первое слово после "dev".
	ISPInterface string
	// QueueNum — номер очереди NFQUEUE (по умолчанию 200, совпадает с firewall.Config.NFQueueNum).
	QueueNum int
	// PolicyName — имя политики (по умолчанию "signcraze").
	PolicyName string
	// Args — основные аргументы DPI-стратегии для TCP.
	Args string
	// ArgsQUIC — аргументы для QUIC/UDP-трафика.
	ArgsQUIC string
	// ArgsUDP — дополнительные аргументы для UDP (пусто по умолчанию).
	ArgsUDP string
	// HostlistPath — путь к файлу со списком доменов для selective DPI.
	// Если пуст — флаг --hostlist не добавляется (desync для всего трафика).
	// Если задан — файл должен существовать на момент запуска nfqws2.
	HostlistPath string
}

// DefaultConfigParams возвращает параметры по умолчанию согласно BEHAVIOR_SPEC §2.
func DefaultConfigParams() ConfigParams {
	return ConfigParams{
		ISPInterface: "eth0",
		QueueNum:     200,
		PolicyName:   "signcraze",
		Args:         "--dpi-desync=fake,split2 --dpi-desync-ttl=5 --dpi-desync-fooling=md5sig",
		ArgsQUIC:     "--dpi-desync=fake --dpi-desync-ttl=5",
		ArgsUDP:      "",
	}
}

// WriteHostlist атомарно записывает список доменов в файл по одному на строку.
// Используется как источник для nfqws2 --hostlist=<path>. Пустой targets
// записывает пустой файл (валидно для nfqws2: список без записей = ничего не
// matches, желательно вместо этого вообще не передавать --hostlist).
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
