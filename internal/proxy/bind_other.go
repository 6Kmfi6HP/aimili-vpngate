//go:build !linux

package proxy

import "net"

func applyBindDevice(dialer *net.Dialer, device string) {}
