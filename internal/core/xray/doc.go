// Package xray реализует core.Core для XTLS/Xray-core.
//
// Источник бинарей: GitHub releases XTLS/Xray-core. Asset формата
// `Xray-linux-{arch}.zip` — содержит единственный исполняемый файл `xray`.
//
// Запуск: `xray run -c <config>`. Проверка конфига: `xray test -c <config>`.
// Версия: первая строка `xray version` имеет формат
// `Xray X.Y.Z (Xray, Penetrates Everything.) Custom (...)`.
//
// Phase B этап: skeleton реализует только paths, version, lifecycle и
// register. Download/Install/Render — отдельными слайсами по мере готовности
// canonical Outbound types.
package xray
