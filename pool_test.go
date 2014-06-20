package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	// "io/ioutil"
	// "log"
	"net"
	"testing"
	"time"

	"github.com/knakk/specs"
)

// func init() {
// 	log.SetOutput(ioutil.Discard)
// }

func initFakeConn(i interface{}) (net.Conn, error) {
	var (
		c fakeTCPConn
		b bytes.Buffer
	)
	bufferWriter := bufio.NewWriter(&b)
	c.ReadWriter = bufio.NewReadWriter(
		bufio.NewReader(bytes.NewBufferString(fmt.Sprintf("result #%v\r", i.(int)))),
		bufferWriter)
	return c, nil
}

func TestConnectionPool(t *testing.T) {
	s := specs.New(t)

	p, err := NewSIPConnPool(2, 2, initFakeConn)
	s.ExpectNil(err)
	s.Expect(2, p.Size())

	conn := p.Get()
	r := bufio.NewReader(conn.c)
	msg, err := r.ReadString('\r')
	s.ExpectNil(err)
	s.Expect("result #1\r", msg)
	p.Release(conn)

	conn2 := p.Get()
	r = bufio.NewReader(conn2.c)
	msg, err = r.ReadString('\r')
	s.ExpectNil(err)
	s.Expect("result #2\r", msg)

	conn = p.Get()
	r = bufio.NewReader(conn.c)
	msg, err = r.ReadString('\r')
	s.Expect(io.EOF, err)

	ch := make(chan SIPConn)
	go func() {
		ch <- p.Get()
	}()
	time.Sleep(time.Millisecond * 10)
	select {
	case <-ch:
		t.Fail()
	default:
		break
	}

	p.Release(conn)
	p.Release(conn2)
	time.Sleep(time.Millisecond * 10)
	select {
	case <-ch:
	default:
		t.Fail()
	}
}
