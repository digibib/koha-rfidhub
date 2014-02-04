package main

import (
	"bytes"
	"encoding/json"
	"log"
	"net"
)

// TCPServer listens for and accepts connections from RFID-units
type TCPServer struct {
	listenAddr  string                     // Host:port to listen at
	connections map[string]*RFIDUnit       // Keyed by the unit's IP address (+port)
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
		log.Fatal("ERR ", err)
	}
	defer ln.Close()

	go srv.handleMessages()

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Println("ERR ", err)
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
			log.Printf("TCP [%v] RFID-unit connected\n", unit.conn.RemoteAddr())
			srv.connections[unit.conn.RemoteAddr().String()] = unit
			srv.broadcast <- UIMessage{
				Type: "CONNECT",
				ID:   unit.conn.RemoteAddr().String()}
		case unit := <-srv.rmChan:
			log.Printf("TCP [%v] RFID-unit disconnected\n", unit.conn.RemoteAddr())
			srv.broadcast <- UIMessage{
				Type: "DISCONNECT",
				ID:   unit.conn.RemoteAddr().String()}
			delete(srv.connections, unit.conn.RemoteAddr().String())
		case msg := <-srv.incoming:
			err := json.Unmarshal(msg, &idMsg)
			if err != nil {
				log.Println("ERR", err)
				break
			}
			unit, ok := srv.connections[idMsg.ID]
			if !ok {
				log.Println("ERR", "Cannot transmit message to missing RFIDunit", idMsg.ID)
				break
			}
			bMsg.Write(idMsg.Msg)
			bMsg.Write([]byte("\n"))
			unit.ToRFID <- bMsg.Bytes()
			log.Println("<-", "UI to", idMsg.ID, string(idMsg.Msg))
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
	defer c.Close()

	srv.addChan <- unit

	defer func() {
		srv.rmChan <- unit
	}()

	go unit.run()
	go unit.tcpWriter()
	unit.tcpReader()
}
