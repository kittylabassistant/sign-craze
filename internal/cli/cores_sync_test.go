package cli

import (
	"sort"
	"testing"

	"github.com/kittylabassistant/sign-craze/internal/core"
	"github.com/kittylabassistant/sign-craze/internal/state"
)

// TestValidCoresMatchesRegistry catches drift между state.ValidCores
// (используется в state.Validate) и core.Names() (фактический registry).
//
// Если кто-то добавил ядро через core.Register(), но забыл обновить
// state.ValidCores — этот тест падает. Без сихнронизации Validate() отклонит
// новое ядро как unknown даже после успешной регистрации, и --core <name>
// перестанет работать.
func TestValidCoresMatchesRegistry(t *testing.T) {
	registered := core.Names()
	declared := append([]string{}, state.ValidCores...)
	sort.Strings(registered)
	sort.Strings(declared)

	if len(registered) != len(declared) {
		t.Fatalf("ValidCores и registry разошлись: ValidCores=%v, registry=%v",
			declared, registered)
	}
	for i := range registered {
		if registered[i] != declared[i] {
			t.Errorf("позиция %d: registry=%q, ValidCores=%q (синхронизируйте state.ValidCores с core.Register() в internal/cli/cores.go)",
				i, registered[i], declared[i])
		}
	}
}
