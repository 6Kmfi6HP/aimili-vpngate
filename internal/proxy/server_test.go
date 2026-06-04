package proxy

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestPlainHTTPProxy(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer target.Close()
	addr, stop := startTestProxy(t, "")
	defer stop()

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_, _ = fmt.Fprintf(conn, "GET %s HTTP/1.1\r\nHost: example.test\r\nConnection: close\r\n\r\n", target.URL)
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if string(body) != "ok" {
		t.Fatalf("body = %q", body)
	}
}

func TestHTTPConnectProxy(t *testing.T) {
	echo, stopEcho := startEcho(t)
	defer stopEcho()
	addr, stop := startTestProxy(t, "")
	defer stop()

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_, _ = fmt.Fprintf(conn, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", echo, echo)
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("CONNECT status = %s", resp.Status)
	}
	_, _ = conn.Write([]byte("ping"))
	buf := make([]byte, 4)
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatal(err)
	}
	if string(buf) != "ping" {
		t.Fatalf("echo = %q", buf)
	}
}

func TestSOCKS5Proxy(t *testing.T) {
	echo, stopEcho := startEcho(t)
	defer stopEcho()
	addr, stop := startTestProxy(t, "")
	defer stop()

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_, _ = conn.Write([]byte{0x05, 0x01, 0x00})
	handshake := make([]byte, 2)
	if _, err := io.ReadFull(conn, handshake); err != nil {
		t.Fatal(err)
	}
	host, port, _ := net.SplitHostPort(echo)
	req := []byte{0x05, 0x01, 0x00, 0x03, byte(len(host))}
	req = append(req, []byte(host)...)
	req = append(req, byte(portNumber(port)>>8), byte(portNumber(port)))
	_, _ = conn.Write(req)
	reply := make([]byte, 10)
	if _, err := io.ReadFull(conn, reply); err != nil {
		t.Fatal(err)
	}
	if reply[1] != 0x00 {
		t.Fatalf("SOCKS reply = %#v", reply)
	}
	_, _ = conn.Write([]byte("pong"))
	buf := make([]byte, 4)
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatal(err)
	}
	if string(buf) != "pong" {
		t.Fatalf("echo = %q", buf)
	}
}

func TestDialerMissingDeviceFailsOnLinux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("SO_BINDTODEVICE is Linux-specific")
	}
	d := Dialer{Timeout: 100 * time.Millisecond, Device: "definitely-missing0"}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := d.DialContext(ctx, "tcp", "127.0.0.1:1")
	if err == nil || !strings.Contains(err.Error(), "no such device") && !strings.Contains(err.Error(), "operation not permitted") {
		t.Fatalf("expected bind-device failure, got %v", err)
	}
}

func startTestProxy(t *testing.T, device string) (string, context.CancelFunc) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := &Server{Dialer: Dialer{Timeout: time.Second, Device: device}}
	go func() {
		_ = s.Serve(ctx, ln)
	}()
	return ln.Addr().String(), cancel
}

func startEcho(t *testing.T) (string, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				_, _ = io.Copy(c, c)
			}(conn)
		}
	}()
	return ln.Addr().String(), func() {
		_ = ln.Close()
		<-done
	}
}

func portNumber(port string) int {
	var p int
	_, _ = fmt.Sscanf(port, "%d", &p)
	return p
}
