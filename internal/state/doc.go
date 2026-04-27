// Package state хранит персистентное состояние sign-craze в /opt/etc/sign-craze/state.json.
//
// State содержит выбранный режим, список outbound-конфигураций sing-box,
// дополнительные проксируемые порты, IP/CIDR-исключения и настройки DPI.
//
// Все мутирующие команды CLI читают State.Load → модифицируют → State.Save.
// Sing-box config.json регенерируется из State при каждом изменении.
//
// Пакет также содержит конкретные имплементации интерфейсов internal/web
// (StatusReader, ConfigRW, PortsManager, ExcludesManager), подключающие
// state.json к Web UI.
package state
