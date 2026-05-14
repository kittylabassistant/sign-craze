// Package naiveproxy управляет жизненным циклом naiveproxy daemon
// (https://github.com/klzgrad/naiveproxy) как supervised peer для sing-box.
//
// Архитектура — process chain (см. ADR-0021-naiveproxy-process-chain):
//  1. naiveproxy скачивается с GitHub Releases (klzgrad/naiveproxy).
//  2. Установка в /opt/sbin/naive (atomic replace через atomicfs).
//  3. Запуск как daemon через service.Lifecycle:
//     naive --listen=socks://127.0.0.1:<NaiveListenPort> \
//     --proxy=https://<user>:<pass>@<server>:<port>
//  4. sing-box получает socks5-outbound к 127.0.0.1:NaiveListenPort.
//
// Альтернатива (native sing-box с -tags with_naive_outbound + libcronet)
// отвергнута из-за несовместимости с CGO_ENABLED=0 multi-arch (mips/mipsle).
//
// klzgrad/naiveproxy НЕ публикует big-endian MIPS (только linux-mipsel) —
// поддержка для GOARCH=mips закрыта в Download.
package naiveproxy
