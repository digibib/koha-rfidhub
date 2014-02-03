package main

import (
	"log"
	"net"
)

// TCPServer listens for and accepts connections from RFID-units
type TCPServer struct {
	listenAddr  string               // Host:port to listen at
	connections map[string]*RFIDUnit // Keyed by the unit's IP address (+port)
	addChan     chan *RFIDUnit       // Register a RFIDUnit
	rmChan      chan *RFIDUnit       // Remove a RFIDUnit
	incoming    chan []byte          // Incoming messages from RFIDUnits from UI
	outgoing    chan []byte          // Outgoing messages to UI
}

// run listens for and accept incomming connections. It is meant to run in
// its own goroutine.
func (srv TCPServer) run() {
	ln, err := net.Listen("tcp", srv.listenAddr)
	if err != nil {
		log.Fatal(err)
	}
	defer ln.Close()

	go srv.handleMessages()

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Println(err)
			continue
		}
		go srv.handleConnection(conn)
	}
}

func newTCPServer(cfg *config) *TCPServer {
	return &TCPServer{
		connections: make(map[string]*RFIDUnit, 0),
		listenAddr:  ":" + cfg.TCPPort,
		addChan:     make(chan *RFIDUnit),
		rmChan:      make(chan *RFIDUnit),
	}
}

func (srv TCPServer) get(addr string) <-chan *RFIDUnit {
	c := make(chan *RFIDUnit)
	for {
		go func() {
			if a, ok := srv.connections[addr]; ok {
				c <- a
				return
			}
		}()
		return c
	}
}

func (srv TCPServer) handleMessages() {
	for {
		select {
		case unit := <-srv.addChan:
			log.Printf("TCP [%v] RFID-unit connected\n", unit.conn.RemoteAddr())
			srv.connections[unit.conn.RemoteAddr().String()] = unit
			uiHub.broadcast <- UIMessage{
				Type: "CONNECT",
				ID:   unit.conn.RemoteAddr().String()}
		case unit := <-srv.rmChan:
			log.Printf("TCP [%v] RFID-unit disconnected\n", unit.conn.RemoteAddr())
			uiHub.broadcast <- UIMessage{
				Type: "DISCONNECT",
				ID:   unit.conn.RemoteAddr().String()}
			delete(srv.connections, unit.conn.RemoteAddr().String())
		}
	}
}

func (srv TCPServer) handleConnection(c net.Conn) {
	unit := newRFIDUnit(c)
	defer c.Close()

	srv.addChan <- unit

	defer func() {
		srv.rmChan <- unit
	}()

	go unit.run()
	go unit.tcpWriter()
	unit.tcpReader()
}
