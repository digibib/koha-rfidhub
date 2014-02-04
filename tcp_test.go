package main

import (
	"net"
	"testing"
	"time"

	"github.com/knakk/specs"
)

func TestTCPServer(t *testing.T) {
	s := specs.New(t)

	cfg := &config{
		TCPPort: "6767",
	}
	srv := newTCPServer(cfg)
	s.Expect(0, len(srv.connections))
	go srv.run()

	// Need to run websocket hub as well, otherwise tcpserver will block
	// trying to broadcast messages
	uiHub = NewHub()
	go uiHub.run()

	c, err := net.Dial("tcp", "localhost:6767")
	s.ExpectNilFatal(err)
	time.Sleep(time.Millisecond * 10)
	s.Expect(1, len(srv.connections))

	_, err = c.Write([]byte("PING\n"))
	s.ExpectNil(err)

	err = c.Close()
	s.ExpectNil(err)
	time.Sleep(time.Millisecond * 10)
	s.Expect(0, len(srv.connections))
}
