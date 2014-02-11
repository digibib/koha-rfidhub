package main

import (
	"bufio"
	"net"
	"testing"
	"time"

	"github.com/knakk/specs"
	"github.com/loggo/loggo"
)

func init() {
	loggo.RemoveWriter("default")
}

func TestTCPServer(t *testing.T) {
	s := specs.New(t)

	cfg := &config{
		TCPPort: "6767",
	}
	srv := newTCPServer(cfg)
	discardChan := make(chan MsgToUI, 10)
	srv.broadcast = discardChan
	go srv.run()
	time.Sleep(time.Millisecond * 10)

	c, err := net.Dial("tcp", "localhost:6767")
	s.ExpectNilFatal(err)

	time.Sleep(time.Millisecond * 10)
	_, err = c.Write([]byte("PING\n"))
	s.ExpectNil(err)

	srv.incoming <- []byte(`{ "IP":"` + addr2IP(c.LocalAddr().String()) + `",
		"Action": "RAW", "RawMsg": {"cmd":"HI-FROM-SERVER!"} }`)
	r := bufio.NewReader(c)
	msg, err := r.ReadString('\n')
	s.ExpectNil(err)
	s.Expect(`{"cmd":"HI-FROM-SERVER!"}`+"\n", msg)

	err = c.Close()
	s.ExpectNil(err)
}
