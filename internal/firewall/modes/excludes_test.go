package modes

import (
	"strings"
	"testing"
)

func TestExcludeRules_Structure(t *testing.T) {
	rules := ExcludeRules()
	if len(rules) != 1 {
		t.Fatalf("ожидалось 1 правило, получено %d", len(rules))
	}
	r := rules[0]
	if r.Table != "mangle" {
		t.Errorf("Table = %q, ожидалось mangle", r.Table)
	}
	if r.Chain != "signcraze" {
		t.Errorf("Chain = %q, ожидалось signcraze", r.Chain)
	}
	args := strings.Join(r.Args, " ")
	if !strings.Contains(args, "--match-set signcraze_excludes dst") {
		t.Errorf("отсутствует match-set: %s", args)
	}
	if !strings.Contains(args, "-j RETURN") {
		t.Errorf("должно быть RETURN: %s", args)
	}
}
