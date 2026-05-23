package main

import (
	"context"
	"net"
	"sync"
)

type ForwardingTunnel struct {
	LocalPort  int
	RemoteHost string
	RemotePort int
	Listener   net.Listener
	Cancel     context.CancelFunc
}

var (
	tunnels   = map[int]*ForwardingTunnel{}
	tunnelsMu sync.Mutex
)
