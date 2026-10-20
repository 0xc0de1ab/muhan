package main

import (
	"bufio"
	"io"
	"net"
	"testing"
)

func TestParseProxyIPHeader(t *testing.T) {
	cases := []struct {
		line string
		want string
		ok   bool
	}{
		{"MUHAN-PROXY-IP 203.0.113.7\r\n", "203.0.113.7", true},
		{"MUHAN-PROXY-IP 203.0.113.7\n", "203.0.113.7", true},
		{"MUHAN-PROXY-IP 2001:db8::1\r\n", "2001:db8::1", true},
		{"MUHAN-PROXY-IP ::1\r\n", "::1", true},
		{"MUHAN-PROXY-IP not-an-ip\r\n", "", false},
		{"MUHAN-PROXY-IP \r\n", "", false},
		{"홍길동\r\n", "", false},
		{"X-Real-IP: 203.0.113.7\r\n", "", false},
		{"", "", false},
	}
	for _, c := range cases {
		got, ok := parseProxyIPHeader(c.line)
		if got != c.want || ok != c.ok {
			t.Errorf("parseProxyIPHeader(%q) = (%q, %v), want (%q, %v)", c.line, got, ok, c.want, c.ok)
		}
	}
}

func TestResolveProxiedRemoteIPHonorsLoopbackHeader(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	accepted := make(chan net.Conn, 1)
	go func() {
		c, err := ln.Accept()
		if err == nil {
			accepted <- c
		}
	}()

	client, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	server := <-accepted
	defer server.Close()

	go func() {
		_, _ = io.WriteString(client, "MUHAN-PROXY-IP 203.0.113.7\r\nhello\n")
	}()

	wrapped, remoteIP := resolveProxiedRemoteIP(server, "127.0.0.1")
	if remoteIP != "203.0.113.7" {
		t.Fatalf("remoteIP = %q, want 203.0.113.7", remoteIP)
	}
	// The header must be consumed; the payload must survive.
	br := bufio.NewReader(wrapped)
	line, err := br.ReadString('\n')
	if err != nil || line != "hello\n" {
		t.Fatalf("post-header read = %q, %v; want %q, nil", line, err, "hello\n")
	}
}

func TestResolveProxiedRemoteIPPushesBackNonHeader(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	accepted := make(chan net.Conn, 1)
	go func() {
		c, err := ln.Accept()
		if err == nil {
			accepted <- c
		}
	}()

	client, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	server := <-accepted
	defer server.Close()

	go func() {
		_, _ = io.WriteString(client, "홍길동\nsecond\n")
	}()

	wrapped, remoteIP := resolveProxiedRemoteIP(server, "127.0.0.1")
	if remoteIP != "127.0.0.1" {
		t.Fatalf("remoteIP = %q, want 127.0.0.1", remoteIP)
	}
	br := bufio.NewReader(wrapped)
	for _, want := range []string{"홍길동\n", "second\n"} {
		line, err := br.ReadString('\n')
		if err != nil || line != want {
			t.Fatalf("read = %q, %v; want %q, nil", line, err, want)
		}
	}
}

type remoteAddrConn struct {
	net.Conn
	addr net.Addr
}

func (c *remoteAddrConn) RemoteAddr() net.Addr { return c.addr }

func TestResolveProxiedRemoteIPIgnoresNonLoopback(t *testing.T) {
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	fake := &remoteAddrConn{Conn: a, addr: &net.TCPAddr{IP: net.ParseIP("203.0.113.7"), Port: 4000}}

	wrapped, remoteIP := resolveProxiedRemoteIP(fake, "203.0.113.7")
	if remoteIP != "203.0.113.7" {
		t.Fatalf("remoteIP = %q, want 203.0.113.7", remoteIP)
	}
	if wrapped != fake {
		t.Fatal("non-loopback conn must be returned unwrapped")
	}
	// Returning at all proves the point: resolving a non-loopback peer must
	// not read from the connection (a pipe read would block with no writer).
}
