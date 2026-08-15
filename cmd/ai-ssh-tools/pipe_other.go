//go:build !windows
// +build !windows

package main

import (
	"fmt"
	"net"
)

func dialWindowsNamedPipe(path string) (net.Conn, error) {
	return nil, fmt.Errorf("named pipes are only supported on windows")
}
