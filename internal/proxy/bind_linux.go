//go:build linux

package proxy

import (
	"net"
	"syscall"
)

func applyBindDevice(dialer *net.Dialer, device string) {
	if device == "" {
		return
	}
	dialer.Control = func(network, address string, c syscall.RawConn) error {
		var controlErr error
		if err := c.Control(func(fd uintptr) {
			controlErr = syscall.SetsockoptString(int(fd), syscall.SOL_SOCKET, syscall.SO_BINDTODEVICE, device)
		}); err != nil {
			return err
		}
		return controlErr
	}
}
