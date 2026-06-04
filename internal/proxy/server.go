package proxy

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Server struct {
	Addr   string
	Dialer Dialer
}

func (s *Server) ListenAndServe(ctx context.Context) error {
	ln, err := net.Listen("tcp", s.Addr)
	if err != nil {
		return err
	}
	return s.Serve(ctx, ln)
}

func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	defer ln.Close()
	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()
	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			continue
		}
		go s.handle(ctx, conn)
	}
}

func (s *Server) handle(ctx context.Context, conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Minute))
	br := bufio.NewReader(conn)
	first, err := br.Peek(1)
	if err != nil {
		return
	}
	if first[0] == 0x05 {
		_ = s.handleSOCKS5(ctx, br, conn)
		return
	}
	_ = s.handleHTTP(ctx, br, conn)
}

func (s *Server) handleHTTP(ctx context.Context, br *bufio.Reader, client net.Conn) error {
	req, err := http.ReadRequest(br)
	if err != nil {
		return err
	}
	if req.Method == http.MethodConnect {
		target, err := s.Dialer.DialContext(ctx, "tcp", req.Host)
		if err != nil {
			_, _ = io.WriteString(client, "HTTP/1.1 502 Bad Gateway\r\n\r\n")
			return err
		}
		defer target.Close()
		_, _ = io.WriteString(client, "HTTP/1.1 200 Connection Established\r\n\r\n")
		relay(client, target)
		return nil
	}
	if req.URL == nil || req.URL.Host == "" {
		_, _ = io.WriteString(client, "HTTP/1.1 400 Bad Request\r\n\r\n")
		return errors.New("proxy request missing absolute URL")
	}
	address := req.URL.Host
	if !strings.Contains(address, ":") {
		if req.URL.Scheme == "https" {
			address += ":443"
		} else {
			address += ":80"
		}
	}
	target, err := s.Dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		_, _ = io.WriteString(client, "HTTP/1.1 502 Bad Gateway\r\n\r\n")
		return err
	}
	defer target.Close()
	req.RequestURI = ""
	req.Header.Del("Proxy-Connection")
	if err := req.Write(target); err != nil {
		return err
	}
	_, err = io.Copy(client, target)
	return err
}

func (s *Server) handleSOCKS5(ctx context.Context, br *bufio.Reader, client net.Conn) error {
	header := make([]byte, 2)
	if _, err := io.ReadFull(br, header); err != nil {
		return err
	}
	methods := make([]byte, int(header[1]))
	if _, err := io.ReadFull(br, methods); err != nil {
		return err
	}
	if _, err := client.Write([]byte{0x05, 0x00}); err != nil {
		return err
	}
	req := make([]byte, 4)
	if _, err := io.ReadFull(br, req); err != nil {
		return err
	}
	if req[1] != 0x01 {
		_, _ = client.Write([]byte{0x05, 0x07, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return errors.New("unsupported socks command")
	}
	host, err := readSOCKSHost(br, req[3])
	if err != nil {
		return err
	}
	portRaw := make([]byte, 2)
	if _, err := io.ReadFull(br, portRaw); err != nil {
		return err
	}
	address := net.JoinHostPort(host, strconv.Itoa(int(binary.BigEndian.Uint16(portRaw))))
	target, err := s.Dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		_, _ = client.Write([]byte{0x05, 0x05, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return err
	}
	defer target.Close()
	if _, err := client.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0}); err != nil {
		return err
	}
	relay(client, target)
	return nil
}

func readSOCKSHost(r io.Reader, atyp byte) (string, error) {
	switch atyp {
	case 0x01:
		buf := make([]byte, 4)
		if _, err := io.ReadFull(r, buf); err != nil {
			return "", err
		}
		return net.IP(buf).String(), nil
	case 0x03:
		var length [1]byte
		if _, err := io.ReadFull(r, length[:]); err != nil {
			return "", err
		}
		buf := make([]byte, int(length[0]))
		if _, err := io.ReadFull(r, buf); err != nil {
			return "", err
		}
		return string(buf), nil
	case 0x04:
		buf := make([]byte, 16)
		if _, err := io.ReadFull(r, buf); err != nil {
			return "", err
		}
		return net.IP(buf).String(), nil
	default:
		return "", fmt.Errorf("unsupported socks address type %d", atyp)
	}
}

func relay(a, b net.Conn) {
	done := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(a, b)
		done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(b, a)
		done <- struct{}{}
	}()
	<-done
}
