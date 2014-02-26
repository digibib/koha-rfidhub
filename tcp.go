package main

import (
	"encoding/json"
	"net"

	"github.com/loggo/loggo"
)

var tcpLogger = loggo.GetLogger("tcp")

// TCPServer listens for and accepts connections from RFID-units
type TCPServer struct {
	listenAddr  string               // Host:port to listen at
	connections map[string]*RFIDUnit // Keyed by the unit's IP address
	addChan     chan *RFIDUnit       // Register a RFIDUnit
	rmChan      chan *RFIDUnit       // Remove a RFIDUnit
	incoming    chan []byte          // Incoming messages (going to) RFIDUnits from UI

	// Channel to broadcast to (normally handled by websocket hub)
	broadcast chan encapsulatedUIMsg
}

// run listens for and accept incomming connections. It is meant to run in
// its own goroutine.
func (srv TCPServer) run() {
	ln, err := net.Listen("tcp", srv.listenAddr)
	if err != nil {
		tcpLogger.Errorf(err.Error())
		panic("Can't start TCP-server. Exiting.")
	}
	defer ln.Close()

	go srv.handleMessages()

	for {
		conn, err := ln.Accept()
		if err != nil {
			tcpLogger.Warningf(err.Error())
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
		incoming:    make(chan []byte),
		broadcast:   make(chan encapsulatedUIMsg),
	}
}

func (srv TCPServer) handleMessages() {
	var uiReq encapsulatedUIMsg
	for {
		select {
		case unit := <-srv.addChan:
			tcpLogger.Infof("RFID-unit connected %v", unit.conn.RemoteAddr())
			var ip = addr2IP(unit.conn.RemoteAddr().String())
			if oldunit, ok := srv.connections[ip]; ok {
				tcpLogger.Warningf("Allready connected RFID-unit from same IP-address; disconnecting and overriding.")
				oldunit.conn.Close()
			}
			srv.connections[ip] = unit
			srv.broadcast <- encapsulatedUIMsg{
				Msg: UIMsg{Action: "CONNECT"},
				IP:  ip}
		case unit := <-srv.rmChan:
			tcpLogger.Infof("RFID-unit disconnected %v", unit.conn.RemoteAddr())
			var ip = addr2IP(unit.conn.RemoteAddr().String())
			srv.broadcast <- encapsulatedUIMsg{
				Msg: UIMsg{Action: "DISCONNECT"},
				IP:  ip}
			delete(srv.connections, ip)
		case msg := <-srv.incoming:
			err := json.Unmarshal(msg, &uiReq)
			if err != nil {
				tcpLogger.Warningf(err.Error())
				break
			}
			unit, ok := srv.connections[uiReq.IP]
			if !ok {
				tcpLogger.Warningf("Cannot transmit message to missing RFIDunit %#v", uiReq.IP)
				break
			}
			unit.FromUI <- uiReq
		}
	}
}

func (srv TCPServer) handleConnection(c net.Conn) {
	unit := newRFIDUnit(c)
	unit.broadcast = srv.broadcast
	defer c.Close()

	srv.addChan <- unit

	defer func() {
		srv.rmChan <- unit
	}()

	go unit.run()
	go unit.tcpWriter()
	unit.tcpReader()
}
