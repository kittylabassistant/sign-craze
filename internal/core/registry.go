package core

import (
	"fmt"
	"sort"
	"sync"
)

// registry — глобальный реестр доступных прокси-ядер.
//
// Конкретные ядра регистрируются через init() в своих пакетах:
//
//	func init() { core.Register(&Adapter{}) }
//
// Доступ к registry — через Get/Names/MustGet. Прямая зависимость cli/cmd на
// конкретное ядро запрещена; используется лишь регистрация по имени.
var (
	regMu sync.RWMutex
	reg   = map[string]Core{}
)

// Register добавляет ядро в реестр. Паника при дублирующем имени —
// программная ошибка, должна быть отловлена тестом.
func Register(c Core) {
	if c == nil {
		panic("core.Register: nil Core")
	}
	name := c.Name()
	if name == "" {
		panic("core.Register: Core.Name() пуст")
	}
	regMu.Lock()
	defer regMu.Unlock()
	if _, ok := reg[name]; ok {
		panic(fmt.Sprintf("core.Register: ядро %q уже зарегистрировано", name))
	}
	reg[name] = c
}

// Get возвращает ядро по имени или ошибку если не зарегистрировано.
//
// Пустое имя name трактуется как "sing-box" (default для старых state.json
// без поля core; см. state.DefaultCore). Это допустимо здесь, чтобы вызывающий
// код не дублировал миграционную логику.
func Get(name string) (Core, error) {
	if name == "" {
		name = "sing-box"
	}
	regMu.RLock()
	defer regMu.RUnlock()
	c, ok := reg[name]
	if !ok {
		return nil, fmt.Errorf("core: ядро %q не зарегистрировано (доступно: %v)", name, names())
	}
	return c, nil
}

// MustGet — Get + panic при отсутствии. Использовать только в тестах
// или там где невозможность найти ядро = повреждённый бинарь.
func MustGet(name string) Core {
	c, err := Get(name)
	if err != nil {
		panic(err)
	}
	return c
}

// Active возвращает активное ядро по значению из state.Core.
// Эквивалент Get, но с более явным именованием в call site:
//
//	c, err := core.Active(st.Core)
//
// Empty stateCoreName трактуется как DefaultCore (sing-box) — см. Get.
func Active(stateCoreName string) (Core, error) {
	return Get(stateCoreName)
}

// Names возвращает отсортированный список зарегистрированных имён.
func Names() []string {
	regMu.RLock()
	defer regMu.RUnlock()
	return names()
}

// names — внутренний хелпер без блокировки (вызывается под RLock).
func names() []string {
	out := make([]string, 0, len(reg))
	for k := range reg {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// resetForTest очищает реестр. Только для unit-тестов того же пакета.
//
//nolint:unused // используется в registry_test.go
func resetForTest() {
	regMu.Lock()
	defer regMu.Unlock()
	reg = map[string]Core{}
}
