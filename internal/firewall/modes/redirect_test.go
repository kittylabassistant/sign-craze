package modes

import "testing"

func TestRedirectRules_ВозвращаетNil(t *testing.T) {
	// Режим redirect отложен — возвращает nil (не реализован)
	rules := RedirectRules(7895, 0x53)
	if rules != nil {
		t.Errorf("RedirectRules: ожидался nil (не реализовано), получено %d правил", len(rules))
	}
}
