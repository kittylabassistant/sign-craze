package dpi

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateConfig_ЗаписываетКонфиг(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "nfqws2.conf")

	params := ConfigParams{
		ISPInterface: "ppp0",
		QueueNum:     200,
		PolicyName:   "signcraze",
		Args:         "--dpi-desync=fake,split2",
		ArgsQUIC:     "--dpi-desync=fake",
		ArgsUDP:      "",
	}

	if err := GenerateConfig(params, dst); err != nil {
		t.Fatalf("GenerateConfig: %v", err)
	}

	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	content := string(data)

	checks := []string{
		"ISP_INTERFACE=ppp0",
		"NFQWS_QUEUE_NUM=200",
		"POLICY_NAME=signcraze",
		"--dpi-desync=fake,split2",
	}
	for _, c := range checks {
		if !strings.Contains(content, c) {
			t.Errorf("конфиг не содержит %q", c)
		}
	}
}

func TestGenerateConfig_ОшибкаПустойИнтерфейс(t *testing.T) {
	params := DefaultConfigParams()
	params.ISPInterface = ""
	err := GenerateConfig(params, filepath.Join(t.TempDir(), "nfqws2.conf"))
	if err == nil {
		t.Error("ожидалась ошибка при пустом ISPInterface")
	}
}

func TestGenerateConfig_ОшибкаНулевойQueueNum(t *testing.T) {
	params := DefaultConfigParams()
	params.QueueNum = 0
	err := GenerateConfig(params, filepath.Join(t.TempDir(), "nfqws2.conf"))
	if err == nil {
		t.Error("ожидалась ошибка при QueueNum=0")
	}
}

func TestDefaultConfigParams_ЗначенияИзСпецификации(t *testing.T) {
	p := DefaultConfigParams()
	if p.ISPInterface != "eth0" {
		t.Errorf("ISPInterface = %q, ожидался eth0", p.ISPInterface)
	}
	if p.QueueNum != 200 {
		t.Errorf("QueueNum = %d, ожидался 200", p.QueueNum)
	}
	if p.PolicyName != "signcraze" {
		t.Errorf("PolicyName = %q, ожидался signcraze", p.PolicyName)
	}
}

func TestDetectISPInterface_НаходитИнтерфейс(t *testing.T) {
	cases := []struct {
		name   string
		output string
		want   string
	}{
		{
			name:   "стандартный вывод eth0",
			output: "default via 192.168.1.1 dev eth0 proto dhcp src 192.168.1.100 metric 100",
			want:   "eth0",
		},
		{
			name:   "PPPoE",
			output: "default dev ppp0 scope link",
			want:   "ppp0",
		},
		{
			name:   "несколько строк",
			output: "10.0.0.0/8 dev br0 proto kernel\ndefault via 1.2.3.4 dev wan0",
			want:   "br0",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := DetectISPInterface(tc.output)
			if err != nil {
				t.Fatalf("DetectISPInterface: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDetectISPInterface_ОшибкаПриПустомВыводе(t *testing.T) {
	_, err := DetectISPInterface("")
	if err == nil {
		t.Error("ожидалась ошибка при пустом выводе")
	}
}
