package main

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"strings"
)

// TODO monitoring? what if a connection is lost? how to detect?
// fn Pool.LostConn()
//   pool.size--
//   pool.conn <- InitNewConn()
//   pool.size++

// ConnPool keeps a pool of <size> TCP connections
type ConnPool struct {
	size int
	conn chan net.Conn
}

// InitFunction is the function to initalize a connection before adding it to
// the pool.
type InitFunction func(interface{}) (net.Conn, error)

func initSIPConn(i interface{}) (net.Conn, error) {
	conn, err := net.Dial("tcp", cfg.SIPServer)
	if err != nil {
		return nil, err
	}

	out := fmt.Sprintf(sipMsg93, i.(int), i.(int))
	_, err = conn.Write([]byte(out))
	if err != nil {
		sipLogger.Errorf(err.Error())
		return nil, err
	}
	sipLogger.Infof("-> %v", strings.Trim(out, "\n\r"))

	reader := bufio.NewReader(conn)
	in, err := reader.ReadString('\r')
	if err != nil {
		sipLogger.Errorf(err.Error())
		return nil, err
	}

	// fail if response == 940 (success == 941)
	if in[2] == '0' {
		return nil, errors.New("SIP login failed")
	}

	sipLogger.Infof("<- %v", strings.Trim(in, "\n\r"))
	return conn, nil

}

// Init sets up <size> connections
func (p *ConnPool) Init(size int, initFn InitFunction) {
	p.conn = make(chan net.Conn, size)
	var count = 0
	for i := 1; i <= size; i++ {
		conn, err := initFn(i)
		if err != nil {
			continue
		}
		count++
		p.conn <- conn
	}
	p.size = count
}

// NewSIPConnPool creates a new pool with <size> SIP connections.
func NewSIPConnPool(size int) *ConnPool {
	p := &ConnPool{}
	p.Init(size, initSIPConn)
	return p
}

// Get a connection from the pool.
func (p *ConnPool) Get() net.Conn {
	return <-p.conn
}

// Release returns the connection back to the pool.
func (p *ConnPool) Release(c net.Conn) {
	p.conn <- c
}
