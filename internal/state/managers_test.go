package state

import (
	"context"
	"path/filepath"
	"testing"
)

func TestPortsManager_AddListDelete(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	pm := NewPortsManager(path)
	ctx := context.Background()

	if err := pm.AddPort(ctx, 80); err != nil {
		t.Fatalf("AddPort: %v", err)
	}
	if err := pm.AddPort(ctx, 443); err != nil {
		t.Fatalf("AddPort: %v", err)
	}
	if err := pm.AddPort(ctx, 80); err != nil { // идемпотентно
		t.Fatalf("дубликат AddPort: %v", err)
	}

	list, err := pm.ListPorts(ctx)
	if err != nil {
		t.Fatalf("ListPorts: %v", err)
	}
	if len(list) != 2 || list[0] != 80 || list[1] != 443 {
		t.Errorf("ListPorts = %v, ожидалось [80 443]", list)
	}

	if err := pm.DeletePort(ctx, 80); err != nil {
		t.Fatalf("DeletePort: %v", err)
	}
	list, _ = pm.ListPorts(ctx)
	if len(list) != 1 || list[0] != 443 {
		t.Errorf("после удаления = %v, ожидалось [443]", list)
	}
}

func TestPortsManager_InvalidPort(t *testing.T) {
	pm := NewPortsManager(filepath.Join(t.TempDir(), "s.json"))
	if err := pm.AddPort(context.Background(), 0); err == nil {
		t.Error("ожидалась ошибка для порта 0")
	}
	if err := pm.AddPort(context.Background(), 70000); err == nil {
		t.Error("ожидалась ошибка для порта > 65535")
	}
}

func TestExcludesManager_AddListDelete(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	// Стартуем с пустым Excludes, чтобы изолировать тест от DefaultExcludes.
	if err := Save(path, &State{Excludes: []string{}}); err != nil {
		t.Fatalf("Save initial: %v", err)
	}
	em := NewExcludesManager(path)
	ctx := context.Background()

	if err := em.AddExclude(ctx, "10.0.0.0/8"); err != nil {
		t.Fatalf("AddExclude CIDR: %v", err)
	}
	// Одиночный IP должен преобразоваться в /32.
	if err := em.AddExclude(ctx, "1.1.1.1"); err != nil {
		t.Fatalf("AddExclude IP: %v", err)
	}

	list, _ := em.ListExcludes(ctx)
	if len(list) != 2 {
		t.Errorf("ListExcludes = %v, ожидалось 2 элемента", list)
	}

	if err := em.AddExclude(ctx, "не-cidr"); err == nil {
		t.Error("ожидалась ошибка для некорректного CIDR")
	}

	if err := em.DeleteExclude(ctx, "10.0.0.0/8"); err != nil {
		t.Fatalf("DeleteExclude: %v", err)
	}
	list, _ = em.ListExcludes(ctx)
	if len(list) != 1 || list[0] != "1.1.1.1/32" {
		t.Errorf("после удаления = %v", list)
	}
}

// TestPortsManager_ConcurrentAdd_NoLostUpdates — проверяет flock-сериализацию
// для одновременных AddPort из нескольких goroutine. Без flock последний writer
// затирал бы предыдущие изменения (TOCTOU race в Load→mutate→Save).
func TestPortsManager_ConcurrentAdd_NoLostUpdates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	pm := NewPortsManager(path)
	ctx := context.Background()

	const N = 20
	errCh := make(chan error, N)
	for i := 0; i < N; i++ {
		port := 10000 + i
		go func() { errCh <- pm.AddPort(ctx, port) }()
	}
	for i := 0; i < N; i++ {
		if err := <-errCh; err != nil {
			t.Errorf("параллельный AddPort: %v", err)
		}
	}

	list, err := pm.ListPorts(ctx)
	if err != nil {
		t.Fatalf("ListPorts: %v", err)
	}
	if len(list) != N {
		t.Errorf("len(ports) = %d, ожидалось %d (потеряны записи?): %v", len(list), N, list)
	}
}

func TestParsedExcludes(t *testing.T) {
	s := &State{Excludes: []string{"10.0.0.0/8", "192.168.0.0/16"}}
	pfxs, err := ParsedExcludes(s)
	if err != nil {
		t.Fatalf("ParsedExcludes: %v", err)
	}
	if len(pfxs) != 2 {
		t.Errorf("ожидалось 2 префикса, получено %d", len(pfxs))
	}

	bad := &State{Excludes: []string{"not-a-cidr"}}
	if _, err := ParsedExcludes(bad); err == nil {
		t.Error("ожидалась ошибка")
	}
}
