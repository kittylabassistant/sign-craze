package state

import (
	"runtime"
	"testing"

	"github.com/kittylabassistant/sign-craze/pkg/types"
)

func TestValidate_NaiveSingleOK(t *testing.T) {
	s := Default()
	s.Outbounds = append(s.Outbounds, types.Outbound{
		Tag: "naive-out", Type: "socks", Server: "ex.com", Port: 443,
		Protocol: types.ProtocolNaive,
		Proto: &types.ProtoOpts{
			NaiveUsername: "u", NaivePassword: "p", NaiveListenPort: 18443,
		},
	})
	if runtime.GOARCH == "mips" {
		// на BE MIPS ожидаем отказ
		if err := s.Validate(); err == nil {
			t.Error("ожидалась ошибка на mips BE")
		}
		return
	}
	if err := s.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestValidate_NaiveMultiRejected(t *testing.T) {
	s := Default()
	for i := 0; i < 2; i++ {
		s.Outbounds = append(s.Outbounds, types.Outbound{
			Tag: "n" + string(rune('0'+i)), Type: "socks", Server: "ex.com", Port: 443,
			Protocol: types.ProtocolNaive,
			Proto:    &types.ProtoOpts{NaiveUsername: "u", NaivePassword: "p"},
		})
	}
	if err := s.Validate(); err == nil {
		t.Fatal("ожидалась ошибка про несколько naive outbounds")
	}
}

func TestValidate_NaiveListenPortOutOfRange(t *testing.T) {
	if runtime.GOARCH == "mips" {
		t.Skip("на mips BE сначала упадёт другая проверка")
	}
	s := Default()
	s.Outbounds = append(s.Outbounds, types.Outbound{
		Tag: "naive-out", Type: "socks", Server: "ex.com", Port: 443,
		Protocol: types.ProtocolNaive,
		Proto: &types.ProtoOpts{
			NaiveUsername: "u", NaivePassword: "p", NaiveListenPort: 100,
		},
	})
	if err := s.Validate(); err == nil {
		t.Fatal("ожидалась ошибка про диапазон NaiveListenPort")
	}
}
