package main

import (
	"net"
	"testing"
	//"time"
)

func clientServerInteract() {
	c, err := net.Dial("tcp", "localhost:3333")
	if err != nil {
		return
	}
	defer c.Close()
	_, err = c.Write([]byte("PING\n"))
	if err != nil {
		return
	}
}

func init() {
	cfg := &config{
		TCPPort: "3333",
	}
	srv := newTCPServer(cfg)
	discardChan := make(chan MsgToUI)
	srv.broadcast = discardChan
	go func() {
		for {
			select {
			case <-srv.broadcast:
				//discard message
			}
		}
	}()
	go srv.run()
}

func BenchmarkTCPServerClientChatting(b *testing.B) {
	for i := 0; i < 100; i++ {
		clientServerInteract()
	}
}
