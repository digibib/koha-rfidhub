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
	// fn to initialize the connection
	initFn InitFunction
}

// NewSIPConnPool creates a new pool, given initial and maximum capacity, and
// a factory function
func NewSIPConnPool(initialCap, maxCap int, factory InitFunction) (*ConnPool, error) {
	if initialCap <= 0 || maxCap <= 0 || initialCap > maxCap {
		return nil, errors.New("invalid capacity settings")
	}
	p := &ConnPool{}
	p.initFn = factory
	p.conn = make(chan SIPConn, maxCap)
	for i := 1; i <= initialCap; i++ {
		conn := SIPConn{id: i}
		c, err := p.initFn(i)
		if err != nil {
			return nil, fmt.Errorf("unable to fill the SIP pool: %s", err)
		}
		conn.c = c
		p.conn <- conn
	}
	return p, nil
}

// Get a connection from the pool.
func (p *ConnPool) Get() SIPConn {
	return <-p.conn
}

// Release returns the connection back to the pool.
func (p *ConnPool) Release(c SIPConn) {
	p.conn <- c
}

// Size returns the (aprox) number of connections currently in the pool.
func (p *ConnPool) Size() int {
	return len(p.conn)
}

// initSIPConn is the default factory function for creatin a SIP connection.
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
