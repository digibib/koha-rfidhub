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

	c, err := net.Dial("tcp", "localhost:6767")
	s.ExpectNilFatal(err)
	time.Sleep(time.Millisecond * 10)
	s.Expect(1, len(srv.connections))

	_, err = c.Write([]byte(`{"Cmd":"PING"}`))
	s.ExpectNil(err)
}
