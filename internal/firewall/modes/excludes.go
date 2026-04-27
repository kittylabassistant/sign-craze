package modes

// ExcludeRules возвращает правило bypass: пакеты с dst в ipset signcraze_excludes
// получают RETURN из цепочки signcraze (до достижения MARK-правил).
// Должно вставляться ПЕРВЫМ в signcraze (через iptables -I) или быть единственным правилом
// в одноразовой инициализации; EnsureRule добавляет в конец, поэтому корректность достигается
// порядком вызовов в Applier (excludes Apply — первым, затем mark-правила).
func ExcludeRules() []RuleSpec {
	return []RuleSpec{
		{
			Table: "mangle", Chain: "signcraze",
			Args: []string{
				"-m", "set", "--match-set", "signcraze_excludes", "dst",
				"-j", "RETURN",
				"-m", "comment", "--comment", "signcraze:exclude-bypass",
			},
		},
	}
}
