package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"time"
)

const defaultServerAddr = "127.0.0.1:4000"

type config struct {
	addr           string
	connectTimeout time.Duration
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	cfg, err := parseFlags(args, stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintf(stderr, "muhan-client: %v\n", err)
		return 2
	}

	if err := runClient(cfg, stdin, stdout); err != nil {
		fmt.Fprintf(stderr, "muhan-client: %v\n", err)
		return 1
	}
	return 0
}

func parseFlags(args []string, stderr io.Writer) (config, error) {
	fs := flag.NewFlagSet("muhan-client", flag.ContinueOnError)
	fs.SetOutput(stderr)

	addr := fs.String("addr", defaultServerAddr, "Muhan server TCP address")
	connectTimeout := fs.Duration("connect-timeout", 10*time.Second, "TCP connection timeout")

	if err := fs.Parse(args); err != nil {
		return config{}, err
	}
	if fs.NArg() != 0 {
		return config{}, fmt.Errorf("unexpected arguments: %v", fs.Args())
	}
	if *addr == "" {
		return config{}, errors.New("addr is required")
	}
	if *connectTimeout <= 0 {
		return config{}, errors.New("connect-timeout must be positive")
	}
	return config{addr: *addr, connectTimeout: *connectTimeout}, nil
}

func runClient(cfg config, stdin io.Reader, stdout io.Writer) error {
	if stdin == nil {
		stdin = io.Reader(nil)
	}
	if stdout == nil {
		stdout = io.Discard
	}

	dialer := net.Dialer{Timeout: cfg.connectTimeout}
	conn, err := dialer.Dial("tcp", cfg.addr)
	if err != nil {
		return fmt.Errorf("connect %s: %w", cfg.addr, err)
	}
	defer conn.Close()

	readDone := make(chan error, 1)
	writeDone := make(chan error, 1)

	go func() {
		_, err := io.Copy(stdout, conn)
		readDone <- normalizeCopyError(err)
	}()
	go func() {
		var err error
		if stdin != nil {
			_, err = io.Copy(conn, stdin)
		}
		if tcp, ok := conn.(*net.TCPConn); ok {
			_ = tcp.CloseWrite()
		}
		writeDone <- normalizeCopyError(err)
	}()

	select {
	case err := <-readDone:
		_ = conn.Close()
		return err
	case err := <-writeDone:
		if err != nil {
			_ = conn.Close()
			return err
		}
		err = <-readDone
		_ = conn.Close()
		return err
	}
}

func normalizeCopyError(err error) error {
	if err == nil || errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
		return nil
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) && !opErr.Timeout() {
		return nil
	}
	return err
}
