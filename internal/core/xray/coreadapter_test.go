package xray

import (
	"strings"
	"testing"

	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// TestRenderConfig_GeoRuleSet покрывает баг B1: coreImpl.RenderConfig (публичный
// уровень core.Core-адаптера xray, вызывается из CLI/web-слоя через
// types.CoreRenderParams) обязан учитывать наличие geosite.dat/geoip.dat при
// routing-правилах с geo rule_set'ами. До фикса RenderConfig строил
// p := DefaultConfigParams() и не заполнял p.GeoAssetsDir, из-за чего guard в
// render_rules.go (renderXrayRoutingRules) получал geoAssetsDir="" и полностью
// отключал проверку — пустой GeoAssetsDir трактуется как «проверку файлов не
// делать» (см. doc-комментарий ConfigParams.GeoAssetsDir в render.go). В
// результате Render "успешно" отдавал JSON, а реальный xray падал при старте
// FATAL'ом failed to open file: geosite.dat (инцидент 2026-05-12, см.
// tasks/lessons.md).
func TestRenderConfig_GeoRuleSet(t *testing.T) {
	t.Run("есть_geo_ruleset_dat_файла_нет", func(t *testing.T) {
		// GeoAssetsDir в RenderConfig не параметризуется извне (см. коммент в
		// конце файла) — после фикса он всегда указывает на
		// DefaultConfigDir+"/assets", т.е. /opt/etc/sign-craze/xray/assets.
		// На dev-машине этого пути физически нет — это нормально и ожидаемо,
		// создавать его не нужно: именно так баг B1 проявляется на чистой
		// установке до `--update-geo --core xray`.
		rp := types.CoreRenderParams{
			RoutingConfig: &types.RoutingConfig{
				Version:   1,
				Outbounds: []types.Outbound{{Tag: "direct", Type: "direct"}},
				Rules: []types.RouteRule{
					{RuleSet: []string{"geosite-youtube"}, Outbound: "direct"},
				},
				Final: "direct",
			},
		}

		var c coreImpl
		_, err := c.RenderConfig(rp)
		if err == nil {
			t.Fatal("RenderConfig должен вернуть ошибку: geosite.dat отсутствует, а RoutingConfig содержит geosite rule_set")
		}
		if !strings.Contains(err.Error(), "geosite.dat") {
			t.Errorf("ошибка должна упоминать geosite.dat, получено: %v", err)
		}
		if !strings.Contains(err.Error(), "--update-geo") {
			t.Errorf("ошибка должна содержать подсказку --update-geo, получено: %v", err)
		}
	})

	t.Run("нет_geo_правил_ошибки_быть_не_должно", func(t *testing.T) {
		// RoutingConfig без rule_set'ов (rule фильтрует только по IPCIDR) —
		// guard не должен активироваться независимо от GeoAssetsDir/наличия
		// .dat файлов на диске.
		rp := types.CoreRenderParams{
			RoutingConfig: &types.RoutingConfig{
				Version:   1,
				Outbounds: []types.Outbound{{Tag: "direct", Type: "direct"}},
				Rules: []types.RouteRule{
					{IPCIDR: []string{"10.0.0.0/8"}, Outbound: "direct"},
				},
				Final: "direct",
			},
		}

		var c coreImpl
		if _, err := c.RenderConfig(rp); err != nil {
			t.Fatalf("RenderConfig не должен возвращать ошибку без geo rule_set'ов (GeoAssetsDir может физически отсутствовать): %v", err)
		}
	})

	t.Run("preview_гео_правила_без_dat_успех", func(t *testing.T) {
		// Preview-путь (Web UI /api/preview, Routing Editor) осознанно НЕ
		// требует локальных .dat: rp.Preview=true оставляет GeoAssetsDir
		// пустым и guard выключен. Строгая проверка остаётся только на
		// production-пути (doStart), где Preview=false.
		rp := types.CoreRenderParams{
			Preview: true,
			RoutingConfig: &types.RoutingConfig{
				Version:   1,
				Outbounds: []types.Outbound{{Tag: "direct", Type: "direct"}},
				Rules: []types.RouteRule{
					{RuleSet: []string{"geosite-youtube"}, Outbound: "direct"},
				},
				Final: "direct",
			},
		}

		var c coreImpl
		if _, err := c.RenderConfig(rp); err != nil {
			t.Fatalf("RenderConfig(Preview=true) не должен требовать .dat файлы: %v", err)
		}
	})

	// Кейс "geo rule_set + реально существующие .dat файлы → успех без ошибки"
	// сознательно пропущен на уровне RenderConfig: GeoAssetsDir жёстко зашит на
	// DefaultConfigDir+"/assets" (см. фикс в coreadapter.go) и не
	// параметризуется извне — в types.CoreRenderParams нет соответствующего
	// поля, а RenderConfig не принимает override пути. Расширять публичный
	// контракт CoreRenderParams ради этого теста не требуется и не является
	// частью бага B1. Позитивный сценарий "dat присутствует → успех" уже
	// покрыт на уровне ConfigParams.GeoAssetsDir в render_test.go:
	// TestRender_Geosite_DatPresent_OK.
}
