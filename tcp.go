package main

import (
	"bytes"
	"encoding/json"
	"net"

	"github.com/loggo/loggo"
)

var tcpLogger = loggo.GetLogger("tcp")

// TCPServer listens for and accepts connections from RFID-units
type TCPServer struct {
	listenAddr  string                     // Host:port to listen at
	connections map[string]*RFIDUnit       // Keyed by the unit's IP address
	addChan     chan *RFIDUnit             // Register a RFIDUnit
	rmChan      chan *RFIDUnit             // Remove a RFIDUnit
	incoming    chan []byte                // Incoming messages (going to) RFIDUnits from UI
	outgoing    chan encaspulatedUIMessage // Outgoing messages to UI

	// Channel to broadcast to (normally handled by websocket hub)
	broadcast chan UIMessage
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
		outgoing:    make(chan encaspulatedUIMessage),
		broadcast:   make(chan UIMessage),
	}
}

func (srv TCPServer) handleMessages() {
	var (
		idMsg encaspulatedUIMessage
		bMsg  bytes.Buffer
	)
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
			srv.broadcast <- UIMessage{
				Type: "CONNECT",
				ID:   ip}
		case unit := <-srv.rmChan:
			tcpLogger.Infof("RFID-unit disconnected %v", unit.conn.RemoteAddr())
			var ip = addr2IP(unit.conn.RemoteAddr().String())
			srv.broadcast <- UIMessage{
				Type: "DISCONNECT",
				ID:   ip}
			delete(srv.connections, ip)
		case msg := <-srv.incoming:
			err := json.Unmarshal(msg, &idMsg)
			if err != nil {
				tcpLogger.Warningf(err.Error())
				break
			}
			unit, ok := srv.connections[idMsg.ID]
			if !ok {
				tcpLogger.Warningf("Cannot transmit message to missing RFIDunit %#v", idMsg.ID)
				break
			}
			if !idMsg.PassUnparsed {
				// TODO message handling logic, SIP switch etc
				break
			}
			bMsg.Write(idMsg.Msg)
			bMsg.Write([]byte("\n"))
			unit.ToRFID <- bMsg.Bytes()
			tcpLogger.Infof("<- UI to %v %v", idMsg.ID, string(idMsg.Msg))
			bMsg.Reset()
		case msg := <-srv.outgoing:
			srv.broadcast <- UIMessage{
				ID:      msg.ID,
				Type:    "INFO",
				Message: &msg.Msg,
			}
		}
	}
}

func (srv TCPServer) handleConnection(c net.Conn) {
	unit := newRFIDUnit(c)
	unit.broadcast = srv.outgoing
	defer c.Close()

	srv.addChan <- unit

	defer func() {
		srv.rmChan <- unit
	}()

	go unit.run()
	go unit.tcpWriter()
	unit.tcpReader()
}
