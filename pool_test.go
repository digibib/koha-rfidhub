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

	p := &ConnPool{}
	p.Init(2, initFakeConn)
	s.Expect(2, p.size)

	c := p.Get()
	r := bufio.NewReader(c)
	msg, err := r.ReadString('\r')
	s.ExpectNil(err)
	s.Expect("result #1\r", msg)
	p.Release(c)

	c2 := p.Get()
	r = bufio.NewReader(c2)
	msg, err = r.ReadString('\r')
	s.ExpectNil(err)
	s.Expect("result #2\r", msg)

	c = p.Get()
	r = bufio.NewReader(c)
	msg, err = r.ReadString('\r')
	s.Expect(io.EOF, err)

	ch := make(chan net.Conn)
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

	p.Release(c)
	p.Release(c2)
	time.Sleep(time.Millisecond * 10)
	select {
	case <-ch:
	default:
		t.Fail()
	}

}
