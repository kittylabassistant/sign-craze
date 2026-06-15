// Package proxyparse преобразует proxy-URL в types.Outbound + types.Canonical.
//
// ParseCanonical поддерживает 13 схем:
//   - vless://uuid@host:port?params
//   - vmess://base64(json)
//   - ss://method:password@host:port  или  ss://base64(method:password)@host:port
//   - trojan://password@host:port?params
//   - hysteria2://password@host:port?params  (алиас hy2://)
//   - tuic://uuid:password@host:port?params
//   - socks5://[user:pass@]host:port  (алиас socks://)
//   - http://[user:pass@]host:port
//   - https://[user:pass@]host:port
//   - mierus://user:pass@host?port=N&protocol=TCP|UDP  (human-readable mieru)
//   - mieru://base64(protobuf)  (полный protobuf-config mieru)
//   - naive+https://user:pass@host:port?params
//   - naive+quic://user:pass@host:port?params
//
// Parse является legacy-функцией (обратная совместимость). Для новых вызовов
// использовать ParseCanonical — возвращает Outbound + Canonical с полной
// структурой transport/TLS/proto.
//
// Используется wizard'ом команды `--install`. Парсит то, что вбил пользователь;
// тонкости конфигурации (TLS, transport, flow) можно настроить через UI или
// прямой правкой /opt/etc/sign-craze/state.json.
package proxyparse
