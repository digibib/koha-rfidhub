package main

import (
	"bufio"
	"encoding/json"
	"net"

	"github.com/gorilla/websocket"
	"github.com/loggo/loggo"
)

var hubLogger = loggo.GetLogger("hub")

// Hub waits for webscoket-connections coming from Koha's user interface.
// For each websocket-connection it attempts to open a TCP-connection to a
// RFID-unit using the same IP-adress as the websocket connection.
// If successfull, a state-machine is started to handle all communications
// between the UI, SIP and the RFID-unit.
type Hub struct {
	// A map of connected UI connections:
	uiConnections map[*uiConn]bool
	// Register a new UI connection:
	uiReg chan *uiConn
	// Unregister a UI connection:
	uiUnReg chan *uiConn
	// Notify of lost TCP connection to RFID
	tcpLost chan *uiConn
}

// newHub creates and returns a new Hub instance.
func newHub() *Hub {
	return &Hub{
		uiConnections: make(map[*uiConn]bool),
		uiReg:         make(chan *uiConn),
		uiUnReg:       make(chan *uiConn),
		tcpLost:       make(chan *uiConn),
	}
}

// run starts the Hub. Meant to be run in its own goroutine.
func (h *Hub) run() {
	for {
		select {
		case c := <-h.uiReg:
			// TODO check if connnection allready exist from that IP
			h.uiConnections[c] = true

			var ip = addr2IP(c.ws.RemoteAddr().String())
			hubLogger.Infof("UI[%v] connected", ip)

			// Try to create a TCP connection to RFID-unit:
			conn, err := net.Dial("tcp", ip+":"+cfg.TCPPort)
			if err != nil {
				hubLogger.Warningf("RFID-unit[%v:%v] connection failed: %v", ip, cfg.TCPPort, err.Error())
				// Note that the Hub never retries to connect after failure.
				// The User must refresh the UI page to try to establish the
				// RFID TCP connection again.
				c.send <- UIMsg{Action: "CONNECT", RFIDError: true}
				break
			}

			// Init the RFID-unit with version command
			var initError string
			unit := newRFIDUnit(conn, c.send)
			req := unit.vendor.GenerateRFIDReq(RFIDReq{Cmd: cmdInitVersion})
			_, err = conn.Write(req)
			if err != nil {
				initError = err.Error()
			}
			hubLogger.Infof("-> RFID-unit[%v:%v] %q", ip, cfg.TCPPort, req)

			rdr := bufio.NewReader(conn)
			msg, err := rdr.ReadBytes('\r')
			if err != nil {
				initError = err.Error()
			}
			r, err := unit.vendor.ParseRFIDResp(msg)
			if err != nil {
				initError = err.Error()
			}
			hubLogger.Infof("<- RFID-unit[%v:%v] %q", ip, cfg.TCPPort, msg)

			if !r.OK {
				initError = "RFID-unit responded with NOK"
			}

			if initError != "" {
				hubLogger.Errorf("RFID-unit[%v:%v] initialization failed: %v", ip, cfg.TCPPort, initError)
				c.send <- UIMsg{Action: "CONNECT", RFIDError: true}
				unit = nil
				break
			}

			hubLogger.Infof("RFID-unit[%v:%v] connected & initialized", ip, cfg.TCPPort)
			// Initialize the RFID-unit state-machine with the TCP connection:
			c.unit = unit
			go unit.run()
			go unit.tcpWriter()
			go unit.tcpReader()
			// Notify UI of success:
			c.send <- UIMsg{Action: "CONNECT"}
		case c := <-h.tcpLost:
			// Notify the UI of lost connection to the RFID-unit:
			c.send <- UIMsg{Action: "CONNECT", RFIDError: true}
			// Shutdown the RFID-unit state-machine:
			// c.unit.Quit <- true
			// c.unit = nil
		case c := <-h.uiUnReg:
			// TODO I shouldnt have to do this; but got panic because
			//      "close of closed channel" on some occations.
			if _, ok := h.uiConnections[c]; !ok {
				break
			}
			// Shutdown RFID-unit state-machine if it exists:
			if c.unit != nil {
				c.unit.Quit <- true
			}
			//srv.stopChan <- addr2IP(c.ws.RemoteAddr().String())
			delete(h.uiConnections, c)
			close(c.send)
			hubLogger.Infof("UI[%v] connection lost", addr2IP(c.ws.RemoteAddr().String()))
			c.unit = nil
			// case msg := <-h.broadcast:
			// 	hubLogger.Infof("-> UI %+v", msg)
			// 	for c := range h.uiConnections {
			// 		if (c.ipFilter == "") || (c.ipFilter == addr2IP(msg.IP)) {
			// 			select {
			// 			case c.send <- msg.Msg:
			// 			default:
			// 				hubLogger.Infof("UI[%v] connection lost", addr2IP(c.ws.RemoteAddr().String()))
			// 				//srv.stopChan <- addr2IP(c.ws.RemoteAddr().String())
			// 				close(c.send)
			// 				delete(h.uiConnections, c)
			// 			}
			// 		}
			// 	}
		}
	}
}

// uiConn represents a UI connection. It also stores a reference to the RFID-
// unit state-machine.
type uiConn struct {
	// Websocket connection:
	ws *websocket.Conn
	// RFID-unit state-machine:
	unit *RFIDUnit
	// Outgoing messages to UI:
	send chan UIMsg
}

func (c *uiConn) writer() {
	for message := range c.send {
		err := c.ws.WriteJSON(message)
		if err != nil {
			break
		}
		hubLogger.Infof("-> UI[%v] %+v", addr2IP(c.ws.RemoteAddr().String()), message)
	}
}

func (c *uiConn) reader() {
	for {
		_, msg, err := c.ws.ReadMessage()
		if err != nil {
			break
		}
		var m UIMsg
		err = json.Unmarshal(msg, &m)
		if err != nil {
			continue
		}
		hubLogger.Infof("<- UI[%v] %q", addr2IP(c.ws.RemoteAddr().String()), msg)
		// TODO should block until unit is ready, how?
		if c.unit != nil {
			c.unit.FromUI <- m
		}
	}
}
