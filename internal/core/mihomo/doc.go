// Package mihomo реализует core.Core для MetaCubeX/mihomo (Clash.Meta).
//
// Источник бинарей: GitHub releases MetaCubeX/mihomo. Asset формата
// `mihomo-linux-{arch}[-softfloat]-vX.Y.Z.gz` — GZIP-стрим прямо с ELF-бинарём
// внутри (без TAR-обёртки, в отличие от sing-box).
//
// Запуск: `mihomo -d <config-dir>` (mihomo читает config.yaml из директории,
// не из файла как sing-box/xray). Проверка: `mihomo -t -d <config-dir>`.
// Версия: первая строка `mihomo -v` имеет формат
// `Mihomo Meta vX.Y.Z {go-version} {go-os}/{go-arch} {build-time}`.
//
// Phase C этап: skeleton реализует только paths, version, lifecycle и register.
// Render config (YAML), Clash API reverse-proxy — отдельными слайсами.
package mihomo
