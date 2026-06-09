package main

import (
	"bytes"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

func TestParseFlags(t *testing.T) {
	var stderr bytes.Buffer
	cfg, err := parseFlags([]string{
		"-addr", "127.0.0.1:4040",
		"-connect-timeout", "2s",
	}, &stderr)
	if err != nil {
		t.Fatalf("parse flags: %v", err)
	}
	if cfg.addr != "127.0.0.1:4040" {
		t.Fatalf("addr = %q, want 127.0.0.1:4040", cfg.addr)
	}
	if cfg.connectTimeout != 2*time.Second {
		t.Fatalf("connect timeout = %v, want 2s", cfg.connectTimeout)
	}
}

func TestParseFlagsRejectsUnexpectedArgs(t *testing.T) {
	var stderr bytes.Buffer
	_, err := parseFlags([]string{"extra"}, &stderr)
	if err == nil || !strings.Contains(err.Error(), "unexpected arguments") {
		t.Fatalf("error = %v, want unexpected arguments", err)
	}
}

func TestRunClientCopiesTerminalAndSocketStreams(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	serverDone := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			serverDone <- err
			return
		}
		defer conn.Close()

		if _, err := io.WriteString(conn, "당신의 이름은 무엇입니까? "); err != nil {
			serverDone <- err
			return
		}
		buf := make([]byte, len("인제로\n"))
		if _, err := io.ReadFull(conn, buf); err != nil {
			serverDone <- err
			return
		}
		if string(buf) != "인제로\n" {
			serverDone <- io.ErrUnexpectedEOF
			return
		}
		if _, err := io.WriteString(conn, "암호를 넣어 주십시요: "); err != nil {
			serverDone <- err
			return
		}
		serverDone <- nil
	}()

	var stdout bytes.Buffer
	cfg := config{addr: listener.Addr().String(), connectTimeout: time.Second}
	if err := runClient(cfg, strings.NewReader("인제로\n"), &stdout); err != nil {
		t.Fatalf("run client: %v", err)
	}
	if err := <-serverDone; err != nil {
		t.Fatalf("server: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "당신의 이름은 무엇입니까?") || !strings.Contains(out, "암호를 넣어 주십시요:") {
		t.Fatalf("stdout = %q, want login prompts", out)
	}
}
