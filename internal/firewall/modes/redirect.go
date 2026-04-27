package modes

// RedirectRules возвращает набор правил iptables для режима redirect (NAT REDIRECT).
// Режим redirect использует таблицу nat вместо mangle/TPROXY и не требует поддержки TPROXY
// в ядре, что делает его совместимым со старыми версиями Keenetic.
//
// Реализация отложена до следующей итерации: режим proxy (tproxy) покрывает основной сценарий
// использования на поддерживаемых роутерах.
func RedirectRules(_ uint16, _ uint32) []RuleSpec {
	return nil
}
