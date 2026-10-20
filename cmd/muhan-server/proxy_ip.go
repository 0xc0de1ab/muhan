package main

import (
	"bufio"
	"bytes"
	"io"
	"net"
	"strings"
	"time"
)

// proxyIPHeaderPrefix is the single-line header the bundled websocket proxy
// sends immediately after dialing the game TCP server (issue #14):
//
//	MUHAN-PROXY-IP <client ip>\r\n
//
// The game server honors it ONLY when the socket peer is loopback: the game
// TCP listener may be reachable from the network, so a claimed IP from any
// other peer would let that peer spoof its source address and bypass the
// per-IP connection limit / login lockout keyed by remote IP.
const proxyIPHeaderPrefix = "MUHAN-PROXY-IP "

const (
	// proxyIPReadTimeout bounds how long the accept loop waits for the proxy
	// header. The proxy writes it synchronously right after Dial, so this is
	// generous; it only expires for direct loopback telnet clients, whose
	// bytes are pushed back untouched (see prefixReaderConn).
	proxyIPReadTimeout = 2 * time.Second
	// proxyIPMaxScanBytes bounds the first-line scan so a peer cannot make
	// the accept loop read an unbounded line while looking for the header.
	// A valid header is far shorter than this.
	proxyIPMaxScanBytes = 512
)

// resolveProxiedRemoteIP returns the remote IP the server should attribute
// to conn. For non-loopback peers that is simply the socket peer IP. For
// loopback peers it scans the first line: if it is a well-formed proxy-IP
// header, the claimed IP is used; otherwise the scanned bytes are treated as
// session input and the socket peer IP is kept. Any bytes read during the
// scan are preserved by wrapping conn, so direct telnet clients connecting
// from loopback are unaffected apart from the login prompt waiting for their
// first line (or the short timeout) before proceeding.
func resolveProxiedRemoteIP(conn net.Conn, peerIP string) (net.Conn, string) {
	if ip := net.ParseIP(peerIP); ip == nil || !ip.IsLoopback() {
		return conn, peerIP
	}

	_ = conn.SetReadDeadline(time.Now().Add(proxyIPReadTimeout))
	br := bufio.NewReader(conn)
	scanned, line := scanFirstLine(br, proxyIPMaxScanBytes)
	_ = conn.SetReadDeadline(time.Time{})

	if line != "" {
		if claimed, ok := parseProxyIPHeader(line); ok {
			return &prefixReaderConn{Conn: conn, reader: br}, claimed
		}
	}
	input := io.MultiReader(bytes.NewReader(scanned), br)
	return &prefixReaderConn{Conn: conn, reader: input}, peerIP
}

// scanFirstLine reads from br until the first '\n', an error, or maxBytes,
// whichever comes first. It returns every byte consumed plus the complete
// line ("" when no newline was reached).
func scanFirstLine(br *bufio.Reader, maxBytes int) (scanned []byte, line string) {
	for len(scanned) < maxBytes {
		frag, err := br.ReadSlice('\n')
		scanned = append(scanned, frag...)
		if err == bufio.ErrBufferFull {
			continue
		}
		if err == nil {
			line = string(scanned)
		}
		return scanned, line
	}
	return scanned, ""
}

// parseProxyIPHeader extracts and validates the claimed IP from a
// "MUHAN-PROXY-IP <ip>" line. The claimed value must be a bare IP literal.
func parseProxyIPHeader(line string) (string, bool) {
	claimed, found := strings.CutPrefix(strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r"), proxyIPHeaderPrefix)
	if !found {
		return "", false
	}
	parsed := net.ParseIP(strings.TrimSpace(claimed))
	if parsed == nil {
		return "", false
	}
	return parsed.String(), true
}

// prefixReaderConn serves Reads from reader (bytes already scanned by the
// accept loop) before falling through to the underlying connection, so no
// client input is lost while looking for the proxy header.
type prefixReaderConn struct {
	net.Conn
	reader io.Reader
}

func (c *prefixReaderConn) Read(b []byte) (int, error) {
	return c.reader.Read(b)
}
