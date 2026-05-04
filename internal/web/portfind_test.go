package web

import (
	"errors"
	"net"
	"testing"
)

func TestFindFreePort_FirstFree(t *testing.T) {
	port, l, err := FindFreePort("127.0.0.1", 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	if port == 0 {
		t.Fatal("ожидался ненулевой выбранный порт")
	}
}

func TestFindFreePort_Fallback(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	addr := occupied.Addr().(*net.TCPAddr)
	port, l, err := FindFreePort("127.0.0.1", uint16(addr.Port), 5)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	if port == uint16(addr.Port) {
		t.Fatalf("ожидался порт != %d, получен %d", addr.Port, port)
	}
}

func TestFindFreePort_Exhausted(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	addr := occupied.Addr().(*net.TCPAddr)
	_, _, err = FindFreePort("127.0.0.1", uint16(addr.Port), 1)
	if err == nil {
		t.Fatal("ожидалась ошибка: единственный candidate занят")
	}
	if !errors.Is(err, ErrNoFreePort) {
		t.Fatalf("ожидался ErrNoFreePort, получено: %v", err)
	}
}
