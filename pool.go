package main

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"strings"
)

// SIPConn represents a TCP-connection to the SIP-server. The id is used
// to generate a username/password pair, which must be present in your
// Koha instance'S SIPConfig.xml.
type SIPConn struct {
	id int
	c  net.Conn
}

// InitFunction is the function to initalize a connection before adding it to
// the pool.
type InitFunction func(interface{}) (net.Conn, error)

// ConnPool keeps a pool of n SIPConn to Koha's SIP-server.
type ConnPool struct {
	// read from this channel to get an connection
	conn chan SIPConn
	// send disconnected connections to this channel
	lost chan SIPConn
	// fn to initialize the connection
	initFn InitFunction
}

// Init sets up <size> connections
func (p *ConnPool) Init(size int) {
	p.conn = make(chan SIPConn, size)
	p.lost = make(chan SIPConn, size)
	for i := 1; i <= size; i++ {
		conn := SIPConn{id: i}
		c, err := p.initFn(i)
		if err != nil {
			p.lost <- conn
			continue
		}
		conn.c = c
		p.conn <- conn
	}
}

// NewSIPConnPool creates a new pool with <size> SIP connections. There is no
// guarantee that it succeedes; the user must call Size() to be sure there are
// any.
func NewSIPConnPool(size int) *ConnPool {
	p := &ConnPool{}
	p.initFn = initSIPConn
	p.Init(size)
	return p
}

// Get a connection from the pool.
func (p *ConnPool) Get() SIPConn {
	return <-p.conn
}

// Release returns the connection back to the pool.
func (p *ConnPool) Release(c SIPConn) {
	p.conn <- c
}

// Monitor tries re-connect any disconnected connections. Meant to be run in
// its own goroutine.
func (p *ConnPool) Monitor() {
	for conn := range p.lost {
		c, err := p.initFn(conn.id)
		if err != nil {
			p.lost <- conn
			continue
		}
		conn.c = c
		p.conn <- conn
	}
}

// Size returns the (aprox) number of connections currently in the pool.
func (p *ConnPool) Size() int {
	return len(p.conn)
}

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
	sipLogger.Infof("-> %v", strings.TrimSpace(out))

	reader := bufio.NewReader(conn)
	in, err := reader.ReadString('\r')
	if err != nil {
		sipLogger.Errorf(err.Error())
		return nil, err
	}

	sipLogger.Infof("<- %v", strings.TrimSpace(in))

	// fail if response == 940 (success == 941)
	if in[2] == '0' {
		return nil, errors.New("SIP login failed")
	}

	return conn, nil
}
