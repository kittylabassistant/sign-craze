package core

import "github.com/kittylabassistant/sign-craze/pkg/types"

// DetectResult — итог проверки outbound против одного ядра.
//
// Compatible=true: ядро поддерживает данный outbound (Validate вернул nil).
// Reason — текст ошибки совместимости, пуст при Compatible=true.
type DetectResult struct {
	Name       string
	Compatible bool
	Reason     string
}

// DetectCompatibleCores проверяет outbound против всех зарегистрированных
// ядер. Порядок результата — sorted Names() (mihomo, sing-box, xray).
//
// Используется auto-detect логикой при `--install --proxy <URL>`: если
// `--core` не задан, выбирается ядро по результатам.
func DetectCompatibleCores(ob types.Outbound) []DetectResult {
	names := Names()
	out := make([]DetectResult, 0, len(names))
	for _, n := range names {
		c, err := Get(n)
		if err != nil {
			continue
		}
		r := DetectResult{Name: n}
		if vErr := c.ValidateOutbound(ob); vErr != nil {
			r.Reason = vErr.Error()
		} else {
			r.Compatible = true
		}
		out = append(out, r)
	}
	return out
}

// RecommendCore выбирает рекомендованное ядро для outbound.
//
// Правила:
//   - sing-box если совместим (DefaultCore проекта);
//   - иначе первое совместимое в alphabetical order;
//   - "" если ни одно не совместимо.
//
// allCompatible — имена всех совместимых ядер; используется для multi-match
// info-print в CLI.
func RecommendCore(ob types.Outbound) (recommended string, allCompatible []string) {
	results := DetectCompatibleCores(ob)
	for _, r := range results {
		if r.Compatible {
			allCompatible = append(allCompatible, r.Name)
		}
	}
	if len(allCompatible) == 0 {
		return "", nil
	}
	for _, n := range allCompatible {
		if n == "sing-box" {
			return "sing-box", allCompatible
		}
	}
	return allCompatible[0], allCompatible
}
