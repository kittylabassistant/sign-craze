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

	params := DefaultConfigParams()
	params.ISPInterface = "ppp0"

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
		"NFQWS_QUEUE_NUM=300",
		"POLICY_NAME=signcraze",
		"NFQWS_ARGS=",
		"NFQWS_ARGS_QUIC=",
		"NFQWS_ARGS_UDP=",
		"BLOB_DIR=" + DefaultBlobDir,
		"--lua-desync=fake:blob=tls_clienthello",
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
	if p.QueueNum != DefaultQueueNum {
		t.Errorf("QueueNum = %d, ожидался %d", p.QueueNum, DefaultQueueNum)
	}
	if p.PolicyName != "signcraze" {
		t.Errorf("PolicyName = %q, ожидался signcraze", p.PolicyName)
	}
	if p.BaseArgs == "" {
		t.Error("BaseArgs не должен быть пустым (lua-init + blob-dir)")
	}
	if p.Args == "" || p.ArgsQUIC == "" || p.ArgsUDP == "" {
		t.Error("Args/ArgsQUIC/ArgsUDP должны быть заполнены дефолтами")
	}
}
