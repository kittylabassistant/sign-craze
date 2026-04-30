package modes

import (
	"strings"
	"testing"
)

func TestTProxyRules_СодержитОбязательныеПравила(t *testing.T) {
	rules := TProxyRules(7895, 0x53)

	// Проверяем наличие ключевых правил
	checks := []struct {
		desc    string
		comment string
	}{
		{"mark-ipv4", "signcraze:mark-ipv4"},
		{"mark-ipv6", "signcraze:mark-ipv6"},
		{"tproxy-tcp", "signcraze:tproxy-tcp"},
		{"tproxy-udp", "signcraze:tproxy-udp"},
		{"prerouting-dpi", "signcraze:prerouting-dpi"},
		{"prerouting-mark", "signcraze:prerouting-mark"},
	}

	for _, c := range checks {
		found := false
		for _, r := range rules {
			for _, arg := range r.Args {
				if arg == c.comment {
					found = true
					break
				}
			}
			if found {
				break
			}
		}
		if !found {
			t.Errorf("правило %s (%s) не найдено в наборе", c.desc, c.comment)
		}
	}
}

func TestTProxyRules_ИспользуетКорректныйFWMark(t *testing.T) {
	rules := TProxyRules(7895, 0x53)
	for _, r := range rules {
		for i, arg := range r.Args {
			if (arg == "--set-mark" || arg == "--mark" || arg == "--tproxy-mark") && i+1 < len(r.Args) {
				if r.Args[i+1] != "0x53" {
					t.Errorf("неверный fwmark %q в правиле %v, ожидался 0x53", r.Args[i+1], r.Args)
				}
			}
		}
	}
}

func TestTProxyRules_ИспользуетКорректныйПорт(t *testing.T) {
	rules := TProxyRules(7895, 0x53)
	portFound := false
	for _, r := range rules {
		for i, arg := range r.Args {
			if arg == "--tproxy-port" && i+1 < len(r.Args) {
				if r.Args[i+1] == "7895" {
					portFound = true
				}
			}
		}
	}
	if !portFound {
		t.Error("порт 7895 не найден в tproxy-правилах")
	}
}

func TestTProxyRules_ВсеВТаблицеMangle(t *testing.T) {
	rules := TProxyRules(7895, 0x53)
	for _, r := range rules {
		if r.Table != "mangle" {
			t.Errorf("правило с комментарием %s находится в таблице %q, ожидалась mangle",
				findComment(r.Args), r.Table)
		}
	}
}

func findComment(args []string) string {
	for i, a := range args {
		if a == "--comment" && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

// TestTProxyRules_BypassLoopback проверяет наличие RETURN-bypass правил
// для loopback (127.0.0.0/8), link-local (169.254.0.0/16) и интерфейса lo
// перед TPROXY-правилами — иначе TCP/UDP-стек роутера ломается
// (safety-fixes #6). Раздельные правила вместо `! -s X ! -s Y` в одном
// rule из-за iptables 1.4.21 (Keenetic) не принимающего multiple -s.
func TestTProxyRules_BypassLoopback(t *testing.T) {
	rules := TProxyRules(7895, 0x53)
	wantBypass := map[string]string{
		"signcraze:bypass-loopback-src":  "-s 127.0.0.0/8",
		"signcraze:bypass-linklocal-src": "-s 169.254.0.0/16",
		"signcraze:bypass-lo-iface":      "-i lo",
	}
	found := map[string]bool{}
	for _, r := range rules {
		comment := findComment(r.Args)
		expectArg, ok := wantBypass[comment]
		if !ok {
			continue
		}
		args := strings.Join(r.Args, " ")
		if !strings.Contains(args, expectArg) || !strings.Contains(args, "-j RETURN") {
			t.Errorf("правило %s ожидало %q + `-j RETURN`, получено: %s", comment, expectArg, args)
		}
		found[comment] = true
	}
	for c := range wantBypass {
		if !found[c] {
			t.Errorf("отсутствует bypass-правило %s", c)
		}
	}
}

func TestTProxyRules_ВсеПравилаИмеютКомментарийSigncraze(t *testing.T) {
	rules := TProxyRules(7895, 0x53)
	for _, r := range rules {
		comment := findComment(r.Args)
		if !strings.HasPrefix(comment, "signcraze:") {
			t.Errorf("правило в цепочке %s не имеет комментария signcraze:*, получено %q", r.Chain, comment)
		}
	}
}
