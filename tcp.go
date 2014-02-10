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
	listenAddr  string               // Host:port to listen at
	connections map[string]*RFIDUnit // Keyed by the unit's IP address
	addChan     chan *RFIDUnit       // Register a RFIDUnit
	rmChan      chan *RFIDUnit       // Remove a RFIDUnit
	incoming    chan []byte          // Incoming messages (going to) RFIDUnits from UI

	// Channel to broadcast to (normally handled by websocket hub)
	broadcast chan MsgToUI
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
		broadcast:   make(chan MsgToUI),
	}
}

func (srv TCPServer) handleMessages() {
	var (
		uiReq MsgFromUI
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
			srv.broadcast <- MsgToUI{
				Action: "CONNECT",
				IP:     ip}
		case unit := <-srv.rmChan:
			tcpLogger.Infof("RFID-unit disconnected %v", unit.conn.RemoteAddr())
			var ip = addr2IP(unit.conn.RemoteAddr().String())
			srv.broadcast <- MsgToUI{
				Action: "DISCONNECT",
				IP:     ip}
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

			switch uiReq.Action {
			case "RAW":
				// Pass message unparsed to RFID unit (from test webpage)
				// TODO remove when done testing
				bMsg.Write(*uiReq.RawMsg)
				bMsg.Write([]byte("\n"))
				unit.ToRFID <- bMsg.Bytes()
				tcpLogger.Infof("<- UI raw msg to %v %v", uiReq.IP, string(*uiReq.RawMsg))
				bMsg.Reset()
			}
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
