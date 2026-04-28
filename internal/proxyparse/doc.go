// Package proxyparse преобразует proxy-URL в types.Outbound.
//
// Поддерживаемые форматы:
//   - socks5://[user:pass@]host:port
//   - http(s)://[user:pass@]host:port
//   - vless://uuid@host:port?params
//   - vmess://base64(json)
//   - ss://method:password@host:port  или  ss://base64(method:password)@host:port
//
// Используется wizard'ом команды `--install`. Парсит то, что вбил пользователь;
// тонкости конфигурации (TLS, transport, flow) можно настроить через UI или
// прямой правкой /opt/etc/sign-craze/state.json.
package proxyparse
