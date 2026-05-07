package core

import (
	"errors"
	"reflect"
	"testing"

	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// validatorCore — fakeCore с настраиваемым ValidateOutbound.
// Расширяет fakeCore (registry_test.go) собственной логикой validate.
type validatorCore struct {
	fakeCore
	validateErr error
}

func (v *validatorCore) ValidateOutbound(_ types.Outbound) error {
	return v.validateErr
}

// setupCores регистрирует три ядра с заданными ошибками валидации.
// Имена: "mihomo", "sing-box", "xray" (тот же набор что в production).
func setupCores(t *testing.T, singboxErr, xrayErr, mihomoErr error) {
	t.Helper()
	resetForTest()
	Register(&validatorCore{fakeCore: fakeCore{name: "sing-box"}, validateErr: singboxErr})
	Register(&validatorCore{fakeCore: fakeCore{name: "xray"}, validateErr: xrayErr})
	Register(&validatorCore{fakeCore: fakeCore{name: "mihomo"}, validateErr: mihomoErr})
}

func TestRecommendCore_AllCompatible_PicksSingbox(t *testing.T) {
	setupCores(t, nil, nil, nil)
	defer resetForTest()

	rec, all := RecommendCore(types.Outbound{})
	if rec != "sing-box" {
		t.Errorf("recommended=%q, ожидалось sing-box (default при multi-match)", rec)
	}
	want := []string{"mihomo", "sing-box", "xray"}
	if !reflect.DeepEqual(all, want) {
		t.Errorf("allCompatible=%v, ожидалось %v", all, want)
	}
}

func TestRecommendCore_OnlyXray(t *testing.T) {
	// Кейс PQ-VLESS / XHTTP packet-up: только xray.
	setupCores(t,
		errors.New("sing-box: PQ-VLESS не поддерживается"),
		nil,
		errors.New("mihomo: PQ-VLESS не поддерживается"),
	)
	defer resetForTest()

	rec, all := RecommendCore(types.Outbound{})
	if rec != "xray" {
		t.Errorf("recommended=%q, ожидалось xray (единственный совместимый)", rec)
	}
	if !reflect.DeepEqual(all, []string{"xray"}) {
		t.Errorf("allCompatible=%v, ожидалось [xray]", all)
	}
}

func TestRecommendCore_SingboxAndMihomo(t *testing.T) {
	// Кейс Hysteria2/TUIC: sing-box + mihomo, xray несовместим.
	setupCores(t, nil, errors.New("xray: TUIC не поддерживается"), nil)
	defer resetForTest()

	rec, all := RecommendCore(types.Outbound{})
	if rec != "sing-box" {
		t.Errorf("recommended=%q, ожидалось sing-box (default при наличии)", rec)
	}
	if !reflect.DeepEqual(all, []string{"mihomo", "sing-box"}) {
		t.Errorf("allCompatible=%v, ожидалось [mihomo sing-box]", all)
	}
}

func TestRecommendCore_SingboxAndXray(t *testing.T) {
	// Кейс Vision UDP443: sing-box + xray, mihomo несовместим.
	setupCores(t, nil, nil, errors.New("mihomo: UDP443 не поддерживается"))
	defer resetForTest()

	rec, all := RecommendCore(types.Outbound{})
	if rec != "sing-box" {
		t.Errorf("recommended=%q, ожидалось sing-box", rec)
	}
	if !reflect.DeepEqual(all, []string{"sing-box", "xray"}) {
		t.Errorf("allCompatible=%v, ожидалось [sing-box xray]", all)
	}
}

func TestRecommendCore_MihomoAndXray(t *testing.T) {
	// Кейс Reality+SpiderX (sing-box не поддерживает spider_x):
	// mihomo + xray. sing-box отсутствует → берём первое в алфавите = mihomo.
	setupCores(t, errors.New("sing-box: spider_x не поддерживается"), nil, nil)
	defer resetForTest()

	rec, all := RecommendCore(types.Outbound{})
	// При отсутствии sing-box возвращаем алфавитно первое (mihomo).
	if rec != "mihomo" {
		t.Errorf("recommended=%q, ожидалось mihomo (первое в alphabetical order при отсутствии sing-box)", rec)
	}
	if !reflect.DeepEqual(all, []string{"mihomo", "xray"}) {
		t.Errorf("allCompatible=%v, ожидалось [mihomo xray]", all)
	}
}

func TestRecommendCore_NoneCompatible(t *testing.T) {
	setupCores(t,
		errors.New("sing-box: incompat"),
		errors.New("xray: incompat"),
		errors.New("mihomo: incompat"),
	)
	defer resetForTest()

	rec, all := RecommendCore(types.Outbound{})
	if rec != "" {
		t.Errorf("recommended=%q, ожидалось пустое (никто не совместим)", rec)
	}
	if all != nil {
		t.Errorf("allCompatible=%v, ожидалось nil", all)
	}
}

func TestDetectCompatibleCores_PreservesReason(t *testing.T) {
	// Reason должен присутствовать у incompatible ядер для UX.
	xrayErr := errors.New("xray не поддерживает Hysteria2")
	setupCores(t, nil, xrayErr, nil)
	defer resetForTest()

	results := DetectCompatibleCores(types.Outbound{})
	if len(results) != 3 {
		t.Fatalf("results=%d, ожидалось 3", len(results))
	}
	for _, r := range results {
		if r.Name == "xray" {
			if r.Compatible {
				t.Error("xray должен быть Compatible=false")
			}
			if r.Reason != xrayErr.Error() {
				t.Errorf("xray Reason=%q, ожидалось %q", r.Reason, xrayErr.Error())
			}
		} else {
			if !r.Compatible {
				t.Errorf("%s должен быть Compatible=true", r.Name)
			}
			if r.Reason != "" {
				t.Errorf("%s Reason=%q, ожидалось пусто", r.Name, r.Reason)
			}
		}
	}
}
