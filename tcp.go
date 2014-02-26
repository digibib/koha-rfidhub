package main

import (
	"net"

	"github.com/loggo/loggo"
)

var tcpLogger = loggo.GetLogger("tcp")

// TCPServer listens for and accepts connections from RFID-units
type TCPServer struct {
	listenAddr  string                 // Host:port to listen at
	connections map[string]*RFIDUnit   // Keyed by the unit's IP address
	addChan     chan *RFIDUnit         // Register a RFIDUnit
	rmChan      chan *RFIDUnit         // Remove a RFIDUnit
	stopChan    chan string            // Send END message to RFIDUnit, keyed by IP
	fromUI      chan encapsulatedUIMsg // Incoming messages (going to) RFIDUnits from UI

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
		fromUI:      make(chan encapsulatedUIMsg),
		stopChan:    make(chan string),
		broadcast:   make(chan encapsulatedUIMsg), // TODO rename toUI?
	}
}

func (srv TCPServer) handleMessages() {
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
			// srv.broadcast <- encapsulatedUIMsg{
			// 	Msg: UIMsg{Action: "CONNECT"},
			// 	IP:  ip}
		case unit := <-srv.rmChan:
			tcpLogger.Infof("RFID-unit disconnected %v", unit.conn.RemoteAddr())
			var ip = addr2IP(unit.conn.RemoteAddr().String())
			// srv.broadcast <- encapsulatedUIMsg{
			// 	Msg: UIMsg{Action: "DISCONNECT"},
			// 	IP:  ip}
			delete(srv.connections, ip)
		case msg := <-srv.fromUI:
			unit, ok := srv.connections[msg.IP]
			if !ok {
				tcpLogger.Warningf("Cannot transmit message to missing RFIDunit %#v", msg.IP)
				break
			}
			unit.FromUI <- msg.Msg
		case ip := <-srv.stopChan:
			unit, ok := srv.connections[ip]
			if !ok {
				tcpLogger.Warningf("Cannot transmit message to missing RFIDunit %#v", ip)
				break
			}
			unit.ToRFID <- unit.vendor.GenerateRFIDReq(RFIDReq{Cmd: cmdEndScan})
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
