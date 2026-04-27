package web

import (
	"crypto/sha1" //nolint:gosec // WebSocket handshake требует SHA-1 по RFC 6455
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
)

const wsGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

// wsConn — сырое TCP-соединение после WebSocket-рукопожатия.
type wsConn struct {
	net.Conn
}

// wsUpgrade выполняет WebSocket-рукопожатие (RFC 6455 §4.2.2) и возвращает соединение.
func wsUpgrade(w http.ResponseWriter, r *http.Request) (*wsConn, error) {
	key := r.Header.Get("Sec-Websocket-Key")
	if key == "" {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return nil, errors.New("ws: нет Sec-WebSocket-Key")
	}

	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return nil, errors.New("ws: hijack не поддерживается")
	}

	conn, rw, err := hj.Hijack()
	if err != nil {
		return nil, fmt.Errorf("ws: hijack: %w", err)
	}

	accept := wsAcceptKey(key)
	_, err = fmt.Fprintf(rw,
		"HTTP/1.1 101 Switching Protocols\r\n"+
			"Upgrade: websocket\r\n"+
			"Connection: Upgrade\r\n"+
			"Sec-WebSocket-Accept: %s\r\n\r\n",
		accept)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("ws: отправка 101: %w", err)
	}
	if err := rw.Flush(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("ws: flush 101: %w", err)
	}

	return &wsConn{conn}, nil
}

// SendText отправляет один WebSocket text-фрейм (FIN=1, opcode=0x1).
func (c *wsConn) SendText(data []byte) error {
	header := makeWSHeader(len(data))
	if _, err := c.Conn.Write(header); err != nil {
		return err
	}
	_, err := c.Conn.Write(data)
	return err
}

func makeWSHeader(n int) []byte {
	switch {
	case n <= 125:
		return []byte{0x81, byte(n)}
	case n <= 65535:
		return []byte{0x81, 126, byte(n >> 8), byte(n)}
	default:
		return []byte{
			0x81, 127,
			0, 0, 0, 0,
			byte(n >> 24), byte(n >> 16), byte(n >> 8), byte(n),
		}
	}
}

func wsAcceptKey(key string) string {
	h := sha1.New() //nolint:gosec
	_, _ = io.WriteString(h, key+wsGUID) //nolint:errcheck // hash.Hash.Write never errors
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}
