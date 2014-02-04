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
	discardChan := make(chan UIMessage, 1)
	srv.broadcast = discardChan
	go srv.run()
	time.Sleep(time.Millisecond * 10)

	c, err := net.Dial("tcp", "localhost:6767")
	s.ExpectNilFatal(err)

	_, err = c.Write([]byte("PING\n"))
	s.ExpectNil(err)

	err = c.Close()
	s.ExpectNil(err)
}
