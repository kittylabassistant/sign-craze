package naiveproxy

import (
	"fmt"
	"net/url"

	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// DefaultListenPort — локальный SOCKS5 порт для sing-box → naive bridge.
// MVP: фиксированный 18443. Множественные naive outbounds пока не поддерживаются.
const DefaultListenPort = 18443

// Params задаёт параметры naive daemon.
type Params struct {
	// ListenPort — локальный SOCKS5 порт (127.0.0.1:ListenPort).
	ListenPort int
	// ProxyURL — upstream URL формата https://user:pass@server:port[?padding=true].
	ProxyURL string
	// Padding — включить naive HTTP/2 padding (передаётся как --padding).
	Padding bool
	// ExtraHeaders — дополнительные HTTP-заголовки (--extra-headers).
	ExtraHeaders string
}

// BuildCmdline собирает args для запуска naive (без первого аргумента — бинаря).
func (p Params) BuildCmdline() []string {
	args := []string{
		fmt.Sprintf("--listen=socks://127.0.0.1:%d", p.ListenPort),
		fmt.Sprintf("--proxy=%s", p.ProxyURL),
	}
	if p.Padding {
		args = append(args, "--padding")
	}
	if p.ExtraHeaders != "" {
		args = append(args, "--extra-headers="+p.ExtraHeaders)
	}
	return args
}

// ParamsFromCanonical строит Params из canonical Outbound (Protocol=ProtocolNaive).
// Использует Outbound.Server/Port как upstream и Proto.NaiveUsername/Password.
func ParamsFromCanonical(o types.Outbound) (Params, error) {
	if o.Protocol != types.ProtocolNaive {
		return Params{}, fmt.Errorf("naiveproxy config: ожидался Protocol=naive, получен %q", o.Protocol)
	}
	if o.Proto == nil {
		return Params{}, fmt.Errorf("naiveproxy config: Proto=nil для outbound %q", o.Tag)
	}
	if o.Proto.NaiveUsername == "" {
		return Params{}, fmt.Errorf("naiveproxy config: outbound %q: NaiveUsername не задан", o.Tag)
	}
	if o.Server == "" || o.Port == 0 {
		return Params{}, fmt.Errorf("naiveproxy config: outbound %q: upstream Server/Port не задан", o.Tag)
	}
	listenPort := o.Proto.NaiveListenPort
	if listenPort == 0 {
		listenPort = DefaultListenPort
	}

	// Строим upstream URL вручную, чтобы корректно экранировать username/password.
	userinfo := url.UserPassword(o.Proto.NaiveUsername, o.Proto.NaivePassword)
	upstream := &url.URL{
		Scheme: "https",
		User:   userinfo,
		Host:   fmt.Sprintf("%s:%d", o.Server, o.Port),
	}

	return Params{
		ListenPort:   listenPort,
		ProxyURL:     upstream.String(),
		Padding:      o.Proto.NaivePadding,
		ExtraHeaders: o.Proto.NaiveExtraHdrs,
	}, nil
}
