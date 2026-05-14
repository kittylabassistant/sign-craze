package peer

import (
	"fmt"
	"net"
	"strconv"

	"github.com/kittylabassistant/sign-craze/pkg/types"
)

const (
	// MieruLocalPortBase — стартовый порт для mieru SOCKS5-bridge.
	// Диапазон 40000..49999 за пределами typical-ephemeral, выбран чтобы
	// не пересекаться с динамическими портами Linux (32768..60999).
	MieruLocalPortBase = 40000

	// MieruLocalPortMax — верхняя граница диапазона выделения.
	MieruLocalPortMax = 49999

	// mieruPortPerOutbound — шаг между outbound'ами; даёт по 100 портов
	// каждому, чтобы при повторном выделении после смены индекса не было
	// коллизий с долгоживущими TCP-сокетами в TIME_WAIT.
	mieruPortPerOutbound = 100
)

// AllocateMieruPort выделяет локальный SOCKS5-порт для mieru-outbound
// `outbounds[idx]`. Идемпотентен: если порт уже выделен в Proto.MieruLocalPort,
// возвращает его без изменений.
//
// Алгоритм:
//  1. Если outbounds[idx].Proto.MieruLocalPort > 0 — возвращается без изменений.
//  2. Иначе пробуем порты base+idx*100, base+idx*100+1, ..., пропуская порты,
//     уже занятые другими outbound'ами в state, и порты, которые сейчас занят
//     внешним процессом (probe через net.Listen).
//  3. Если диапазон 40000..49999 исчерпан — возвращается ошибка.
//
// Возвращаемый порт НЕ резервируется (после Close listener'а порт может быть
// занят гонкой). Caller должен сразу запустить peer-процесс, который займёт
// порт окончательно.
func AllocateMieruPort(outbounds []types.Outbound, idx int) (uint16, error) {
	if idx < 0 || idx >= len(outbounds) {
		return 0, fmt.Errorf("peer: AllocateMieruPort: некорректный индекс %d (всего %d)", idx, len(outbounds))
	}
	ob := &outbounds[idx]
	if ob.Protocol != types.ProtocolMieru {
		return 0, fmt.Errorf("peer: AllocateMieruPort: outbound %q не mieru (protocol=%q)", ob.Tag, ob.Protocol)
	}
	if ob.Proto != nil && ob.Proto.MieruLocalPort > 0 {
		return uint16(ob.Proto.MieruLocalPort), nil
	}

	used := collectUsedMieruPorts(outbounds, idx)

	start := MieruLocalPortBase + idx*mieruPortPerOutbound
	if start > MieruLocalPortMax {
		start = MieruLocalPortBase
	}

	for port := start; port <= MieruLocalPortMax; port++ {
		if _, taken := used[port]; taken {
			continue
		}
		if !canBind(port) {
			continue
		}
		// Зафиксировать выделение в outbound.Proto.
		if ob.Proto == nil {
			ob.Proto = &types.ProtoOpts{}
		}
		ob.Proto.MieruLocalPort = port
		return uint16(port), nil
	}

	// Не нашли в верхней половине — попробуем нижнюю.
	for port := MieruLocalPortBase; port < start; port++ {
		if _, taken := used[port]; taken {
			continue
		}
		if !canBind(port) {
			continue
		}
		if ob.Proto == nil {
			ob.Proto = &types.ProtoOpts{}
		}
		ob.Proto.MieruLocalPort = port
		return uint16(port), nil
	}

	return 0, fmt.Errorf("peer: AllocateMieruPort: не удалось выделить локальный SOCKS5-порт для mieru %q (диапазон %d..%d исчерпан)",
		ob.Tag, MieruLocalPortBase, MieruLocalPortMax)
}

// collectUsedMieruPorts собирает все MieruLocalPort, уже выделенные другим
// outbound'ам (исключая текущий idx — он может быть либо 0, либо тот же порт
// при повторном вызове, оба варианта не конфликтуют сами с собой).
func collectUsedMieruPorts(outbounds []types.Outbound, skip int) map[int]struct{} {
	used := make(map[int]struct{})
	for i, ob := range outbounds {
		if i == skip {
			continue
		}
		if ob.Proto == nil || ob.Proto.MieruLocalPort == 0 {
			continue
		}
		used[ob.Proto.MieruLocalPort] = struct{}{}
	}
	return used
}

// canBind пробует кратко занять порт через net.Listen("tcp",
// "127.0.0.1:<port>"). Закрывает listener сразу — порт остаётся свободным
// для немедленного использования peer-процессом.
func canBind(port int) bool {
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return false
	}
	_ = l.Close()
	return true
}

// AllocateAllMieruPorts вызывает AllocateMieruPort для каждого mieru-outbound
// в списке. Outbound'ы других протоколов пропускаются. Возвращает первую ошибку.
//
// Изменения записываются непосредственно в outbounds (slice передаётся по
// ссылке как []types.Outbound, но modify через индекс — стандартный паттерн
// Go). Caller обязан сохранить state.json после вызова.
func AllocateAllMieruPorts(outbounds []types.Outbound) error {
	for i := range outbounds {
		if outbounds[i].Protocol != types.ProtocolMieru {
			continue
		}
		if _, err := AllocateMieruPort(outbounds, i); err != nil {
			return err
		}
	}
	return nil
}
