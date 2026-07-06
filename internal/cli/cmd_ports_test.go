package cli

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/kittylabassistant/sign-craze/internal/state"
)

// captureStdout перехватывает os.Stdout на время выполнения fn и возвращает
// всё, что было напечатано. handlePortAdd/Del/List пишут напрямую в os.Stdout
// через fmt.Println/Printf (не принимают io.Writer), поэтому для проверки
// текста вывода приходится подменять сам os.Stdout.
//
// Тесты, использующие этот хелпер, не должны запускаться параллельно
// (t.Parallel() не вызывается ни в одном тесте пакета cli).
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	orig := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("close pipe writer: %v", err)
	}
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("io.Copy: %v", err)
	}
	return buf.String()
}

// withTempPortsStatePath подменяет portsStatePath на временный файл на время
// теста и возвращает восстанавливающую функцию через t.Cleanup.
// Нужен только для тестов, вызывающих handlePortList напрямую (у него нет
// внешнего withLock — в отличие от handlePortAdd/Del, которые в тестах
// вызываются через извлечённые addPorts/delPorts с явным statePath).
func withTempPortsStatePath(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "state.json")
	orig := portsStatePath
	portsStatePath = path
	t.Cleanup(func() { portsStatePath = orig })
	return path
}

// --- parsePortSpec: чистые юнит-тесты, гонок не касаются ---

func TestParsePortSpec_ОдиночныйПорт(t *testing.T) {
	got, err := parsePortSpec("80")
	if err != nil {
		t.Fatalf("parsePortSpec(80): %v", err)
	}
	if len(got) != 1 || got[0] != 80 {
		t.Errorf("got %v, want [80]", got)
	}
}

func TestParsePortSpec_Диапазон(t *testing.T) {
	got, err := parsePortSpec("1000-1002")
	if err != nil {
		t.Fatalf("parsePortSpec(1000-1002): %v", err)
	}
	want := []uint16{1000, 1001, 1002}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got %v, want %v", got, want)
		}
	}
}

func TestParsePortSpec_Некорректно(t *testing.T) {
	bad := []string{"0", "65536", "abc", "100-50", "0-100", "1-70000", "-5"}
	for _, s := range bad {
		t.Run(s, func(t *testing.T) {
			if _, err := parsePortSpec(s); err == nil {
				t.Errorf("ожидалась ошибка для %q", s)
			}
		})
	}
}

func TestParsePortSpec_ДиапазонПревышаетЛимит(t *testing.T) {
	if _, err := parsePortSpec("1-2000"); err == nil {
		t.Error("ожидалась ошибка для диапазона > maxPortRangeSize")
	}
}

// --- addPorts / delPorts: поведенческие тесты извлечённой логики ---

func TestAddPorts_Идемпотентно(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	ctx := context.Background()

	if err := addPorts(ctx, path, []uint16{80}); err != nil {
		t.Fatalf("addPorts #1: %v", err)
	}
	if err := addPorts(ctx, path, []uint16{80}); err != nil {
		t.Fatalf("addPorts #2 (повтор того же порта): %v", err)
	}

	mgr := state.NewPortsManager(path)
	list, err := mgr.ListPorts(ctx)
	if err != nil {
		t.Fatalf("ListPorts: %v", err)
	}
	if len(list) != 1 || list[0] != 80 {
		t.Errorf("после повторного add = %v, ожидалось [80] (без дублей)", list)
	}
}

func TestAddPorts_Диапазон_РазвёрнутПоштучно(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	ctx := context.Background()

	ports, err := parsePortSpec("100-103")
	if err != nil {
		t.Fatalf("parsePortSpec: %v", err)
	}
	if addErr := addPorts(ctx, path, ports); addErr != nil {
		t.Fatalf("addPorts: %v", addErr)
	}

	mgr := state.NewPortsManager(path)
	list, err := mgr.ListPorts(ctx)
	if err != nil {
		t.Fatalf("ListPorts: %v", err)
	}
	want := []int{100, 101, 102, 103}
	if len(list) != len(want) {
		t.Fatalf("got %v, want %v", list, want)
	}
	for i := range want {
		if list[i] != want[i] {
			t.Errorf("got %v, want %v", list, want)
		}
	}
}

func TestDelPorts_НесуществующийПорт_БезОшибки(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	ctx := context.Background()

	if err := addPorts(ctx, path, []uint16{80}); err != nil {
		t.Fatalf("addPorts: %v", err)
	}
	// Как и PortsManager.DeletePort — удаление отсутствующего порта не ошибка.
	if err := delPorts(ctx, path, []uint16{9999}); err != nil {
		t.Fatalf("delPorts несуществующего порта вернул ошибку: %v", err)
	}

	mgr := state.NewPortsManager(path)
	list, _ := mgr.ListPorts(ctx)
	if len(list) != 1 || list[0] != 80 {
		t.Errorf("список изменился после удаления несуществующего порта: %v", list)
	}
}

func TestDelPorts_УдаляетСуществующий(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	ctx := context.Background()

	if err := addPorts(ctx, path, []uint16{80, 443}); err != nil {
		t.Fatalf("addPorts: %v", err)
	}
	if err := delPorts(ctx, path, []uint16{80}); err != nil {
		t.Fatalf("delPorts: %v", err)
	}

	mgr := state.NewPortsManager(path)
	list, _ := mgr.ListPorts(ctx)
	if len(list) != 1 || list[0] != 443 {
		t.Errorf("после удаления 80 = %v, ожидалось [443]", list)
	}
}

// --- handlePortList: формат вывода (не через withLock, вызывается напрямую) ---

func TestHandlePortList_ФорматВывода(t *testing.T) {
	withTempPortsStatePath(t)
	ctx := context.Background()

	out := captureStdout(t, func() {
		if err := handlePortList(ctx, nil); err != nil {
			t.Fatalf("handlePortList (пусто): %v", err)
		}
	})
	if out != "(пусто)\n" {
		t.Errorf("пустой список: вывод = %q, ожидалось %q", out, "(пусто)\n")
	}

	if err := addPorts(ctx, portsStatePath, []uint16{443, 80, 8080}); err != nil {
		t.Fatalf("addPorts: %v", err)
	}

	out = captureStdout(t, func() {
		if err := handlePortList(ctx, nil); err != nil {
			t.Fatalf("handlePortList: %v", err)
		}
	})
	// ListPorts сортирует по возрастанию (см. internal/state/managers.go) —
	// это соответствует тому, что уже показывает Web UI; CLI теперь не
	// расходится с ним по порядку.
	want := "80\n443\n8080\n"
	if out != want {
		t.Errorf("вывод = %q, ожидалось %q (сортировка по возрастанию, по строке на порт)", out, want)
	}
}

// --- Демонстрация механизма бага B4 и доказательство фикса ---

// TestRawLoadSave_ГонкаСPortsManager_ТеряетПравку детерминированно
// воспроизводит МЕХАНИЗМ бага B4 (без реальных горутин/таймингов — гонка
// воспроизводится точной расстановкой шагов).
//
// Так был устроен cmd_ports.go до фикса: loadState()/saveState() читали и
// писали ВЕСЬ *state.State напрямую, синхронизированные только внешним
// CLI-локом (locks.DefaultPath). Web UI для тех же данных использует
// state.NewPortsManager, который лочит ДРУГОЙ файл (statePath+".lock", см.
// withStateLock в internal/state/managers.go). Эти два лока не пересекаются:
// "сырой" читатель может держать на руках устаревший снимок всего State,
// пока PortsManager успевает применить И сохранить свою правку — и затем
// затереть её целиком при сохранении своего устаревшего снимка.
//
// Этот тест НЕ вызывает код cmd_ports.go (после фикса addPorts/delPorts сами
// используют PortsManager и гонке не подвержены) — он документирует причину
// бага через прямые вызовы internal/state, воспроизводя её надёжно и без
// флуктуаций, в отличие от попытки поймать реальную гонку по таймингу.
func TestRawLoadSave_ГонкаСPortsManager_ТеряетПравку(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	ctx := context.Background()

	if err := state.Save(path, &state.State{Ports: []uint16{80}}); err != nil {
		t.Fatalf("исходное сохранение: %v", err)
	}

	// 1. "Старый cmd_ports.go" читает ВЕСЬ state.json сырым Load — снимок ДО
	//    изменения, которое чуть позже сделает Web UI.
	stale, err := state.Load(path)
	if err != nil {
		t.Fatalf("raw Load: %v", err)
	}

	// 2. Конкурентно (в реальности — из другого запроса/процесса) Web UI
	//    добавляет порт 443 через PortsManager — атомарно, под своим локом.
	mgr := state.NewPortsManager(path)
	if addErr := mgr.AddPort(ctx, 443); addErr != nil {
		t.Fatalf("AddPort через менеджер: %v", addErr)
	}

	// 3. "Старый cmd_ports.go" дописывает 8080 в СВОЙ устаревший снимок и
	//    сохраняет его целиком сырым Save, перезаписывая state.json.
	stale.Ports = append(stale.Ports, 8080)
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
		t.Fatalf("порт 443 не должен был выжить при воспроизведении бага B4 (raw Save должен был его затереть): %v", final.Ports)
	}
	t.Logf("гонка B4 воспроизведена: raw Save затёр правку Web UI, итоговые Ports=%v (443 потерян)", final.Ports)
}

// TestAddPorts_ConcurrentWithPortsManager_НетПотерянныхПортов — прямое
// доказательство фикса B4: реальный код cmd_ports.go (addPorts) и код Web UI
// (state.NewPortsManager напрямую) одновременно пишут в один state.json.
// Так как оба пути теперь используют один и тот же менеджер (а значит один и
// тот же lock-файл statePath+".lock"), ни одна запись не теряется — в отличие
// от TestRawLoadSave_ГонкаСPortsManager_ТеряетПравку выше.
func TestAddPorts_ConcurrentWithPortsManager_НетПотерянныхПортов(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	ctx := context.Background()

	const nCLI = 15 // через addPorts — эмулирует конкурентные sign-craze --port-add
	const nWeb = 15 // через PortsManager напрямую — эмулирует Web UI

	var wg sync.WaitGroup
	errCh := make(chan error, nCLI+nWeb)

	for i := 0; i < nCLI; i++ {
		port := uint16(20000 + i)
		wg.Add(1)
		go func() {
			defer wg.Done()
			errCh <- addPorts(ctx, path, []uint16{port})
		}()
	}

	mgr := state.NewPortsManager(path)
	for i := 0; i < nWeb; i++ {
		port := 30000 + i
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
			t.Errorf("параллельная запись порта: %v", err)
		}
	}

	final, err := state.Load(path)
	if err != nil {
		t.Fatalf("state.Load: %v", err)
	}
	got := make(map[int]bool, len(final.Ports))
	for _, p := range final.Ports {
		got[int(p)] = true
	}
	for i := 0; i < nCLI; i++ {
		if !got[20000+i] {
			t.Errorf("порт %d (через addPorts/CLI) потерян — B4 не исправлен", 20000+i)
		}
	}
	for i := 0; i < nWeb; i++ {
		if !got[30000+i] {
			t.Errorf("порт %d (через PortsManager/Web UI) потерян — B4 не исправлен", 30000+i)
		}
	}
	if len(final.Ports) != nCLI+nWeb {
		t.Errorf("len(Ports) = %d, ожидалось %d: %v", len(final.Ports), nCLI+nWeb, final.Ports)
	}
}
