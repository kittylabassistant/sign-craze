package cli

// Blank-импорты всех прокси-ядер: каждое ядро через init() регистрируется в
// core.registry. CLI использует core.Get(state.Core) для диспетчеризации.
//
// При добавлении нового ядра — добавить здесь blank-import. При удалении —
// убрать. Тесты смотрят core.Names() и должны видеть все три.
import (
	// Регистрация ядер через init() — blank import активирует core.Register.
	_ "github.com/kittylabassistant/sign-craze/internal/core/mihomo"
	_ "github.com/kittylabassistant/sign-craze/internal/core/xray"
	_ "github.com/kittylabassistant/sign-craze/internal/singbox"
)
