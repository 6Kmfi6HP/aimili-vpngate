package proxy

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"strings"
	"time"
)

type Dialer struct {
	Timeout time.Duration
	Device  string
}

func (d Dialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	dialer := &net.Dialer{Timeout: d.Timeout}
	applyBindDevice(dialer, d.Device)
	conn, err := dialer.DialContext(ctx, network, address)
	if err == nil || d.Device == "" || !strings.HasPrefix(network, "tcp") {
		return conn, err
	}
	host, port, splitErr := net.SplitHostPort(address)
	if splitErr != nil || net.ParseIP(host) != nil {
		return nil, err
	}
	ips, resolveErr := d.resolveAOverDevice(ctx, host)
	if resolveErr != nil {
		return nil, errors.Join(err, resolveErr)
	}
	for _, ip := range ips {
		conn, retryErr := dialer.DialContext(ctx, network, net.JoinHostPort(ip, port))
		if retryErr == nil {
			return conn, nil
		}
	}
	return nil, err
}

func (d Dialer) resolveAOverDevice(ctx context.Context, host string) ([]string, error) {
	query, id, err := dnsQuery(host)
	if err != nil {
		return nil, err
	}
	dialer := &net.Dialer{Timeout: d.Timeout}
	applyBindDevice(dialer, d.Device)
	conn, err := dialer.DialContext(ctx, "udp", "8.8.8.8:53")
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	} else if d.Timeout > 0 {
		_ = conn.SetDeadline(time.Now().Add(d.Timeout))
	}
	if _, err := conn.Write(query); err != nil {
		return nil, err
	}
	resp := make([]byte, 1500)
	n, err := conn.Read(resp)
	if err != nil {
		return nil, err
	}
	return parseARecords(resp[:n], id)
}

func dnsQuery(host string) ([]byte, uint16, error) {
	var idRaw [2]byte
	if _, err := io.ReadFull(rand.Reader, idRaw[:]); err != nil {
		return nil, 0, err
	}
	id := binary.BigEndian.Uint16(idRaw[:])
	packet := []byte{idRaw[0], idRaw[1], 0x01, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	for _, part := range strings.Split(host, ".") {
		if part == "" || len(part) > 63 {
			return nil, 0, errors.New("invalid dns name")
		}
		packet = append(packet, byte(len(part)))
		packet = append(packet, []byte(part)...)
	}
	packet = append(packet, 0x00, 0x00, 0x01, 0x00, 0x01)
	return packet, id, nil
}

func parseARecords(resp []byte, id uint16) ([]string, error) {
	if len(resp) < 12 || binary.BigEndian.Uint16(resp[:2]) != id {
		return nil, errors.New("invalid dns response")
	}
	qd := int(binary.BigEndian.Uint16(resp[4:6]))
	an := int(binary.BigEndian.Uint16(resp[6:8]))
	offset := 12
	for i := 0; i < qd; i++ {
		next, err := skipDNSName(resp, offset)
		if err != nil {
			return nil, err
		}
		offset = next + 4
	}
	var ips []string
	for i := 0; i < an && offset < len(resp); i++ {
		next, err := skipDNSName(resp, offset)
		if err != nil {
			return nil, err
		}
		offset = next
		if offset+10 > len(resp) {
			return nil, errors.New("short dns answer")
		}
		typ := binary.BigEndian.Uint16(resp[offset : offset+2])
		class := binary.BigEndian.Uint16(resp[offset+2 : offset+4])
		rdLen := int(binary.BigEndian.Uint16(resp[offset+8 : offset+10]))
		offset += 10
		if offset+rdLen > len(resp) {
			return nil, errors.New("short dns rdata")
		}
		if typ == 1 && class == 1 && rdLen == 4 {
			ips = append(ips, net.IP(resp[offset:offset+4]).String())
		}
		offset += rdLen
	}
	if len(ips) == 0 {
		return nil, errors.New("no A records")
	}
	return ips, nil
}

func skipDNSName(packet []byte, offset int) (int, error) {
	for {
		if offset >= len(packet) {
			return 0, errors.New("dns name out of range")
		}
		length := int(packet[offset])
		offset++
		if length == 0 {
			return offset, nil
		}
		if length&0xC0 == 0xC0 {
			if offset >= len(packet) {
				return 0, errors.New("dns pointer out of range")
			}
			return offset + 1, nil
		}
		offset += length
	}
}
