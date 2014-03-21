package main

import (
	"bufio"
	"encoding/json"
	"fmt"
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
	// Connected IP adresses
	ipAdresses map[string]*uiConn
	// A map of connected UI connections
	uiConnections map[*uiConn]bool
	// Register a new UI connection:
	uiReg chan *uiConn
	// Unregister a UI connection:
	uiUnReg chan *uiConn
}

// newHub creates and returns a new Hub instance.
func newHub() *Hub {
	return &Hub{
		ipAdresses:    make(map[string]*uiConn),
		uiConnections: make(map[*uiConn]bool),
		uiReg:         make(chan *uiConn),
		uiUnReg:       make(chan *uiConn),
	}
}

// run starts the Hub. Meant to be run in its own goroutine.
func (h *Hub) run() {
	for {
		select {
		case c := <-h.uiReg:
			var ip = addr2IP(c.ws.RemoteAddr().String())

			// If there is allready a connection from that IP - close it
			if oldc, ok := h.ipAdresses[ip]; ok {
				hubLogger.Warningf("Duplicate websocket-connection from IP %v; closing the first one.", ip)
				if oldc.unit != nil {
					oldc.unit.Quit <- true
				}

				oldc.unit = nil
				oldc.ws.Close()
				hubLogger.Infof("UI[%v] connection closed", ip)
			}

			h.uiConnections[c] = true
			h.ipAdresses[ip] = c
			hubLogger.Infof("UI[%v] connected", ip)

			// Fail if we don't have any SIP connections in the pool
			if sipPool.Size() == 0 {
				c.send <- UIMsg{Action: "CONNECT", SIPError: true}
				hubLogger.Warningf("Cannot start State-machine for %v: SIP pool is empty", ip)
				break
			}

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

			if initError == "" && !r.OK {
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
		case c := <-h.uiUnReg:
			var ip = addr2IP(c.ws.RemoteAddr().String())

			if _, ok := h.uiConnections[c]; !ok {
				// Connection allready gone. I can't understand how, but...
				break
			}

			// Shutdown RFID-unit state-machine if it exists:
			if c.unit != nil {
				c.unit.Quit <- true
			}

			c.unit = nil
			close(c.send)
			if sameC, ok := h.ipAdresses[ip]; ok {
				if c == sameC {
					delete(h.ipAdresses, ip)
				}
			}
			delete(h.uiConnections, c)
			hubLogger.Infof("UI[%v] connection lost", ip)
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
			hubLogger.Warningf("UI[%v] failed to unmarshal JSON: %q", addr2IP(c.ws.RemoteAddr().String()), msg)
			c.send <- UIMsg{Action: "CONNECT", UserError: true,
				ErrorMessage: fmt.Sprintf("Failed to parse the JSON request: %v", err)}
			continue
		}
		hubLogger.Infof("<- UI[%v] %q", addr2IP(c.ws.RemoteAddr().String()), msg)
		if c.unit != nil {
			if c.unit.state == UNITOff {
				c.unit = nil
				continue
			}
			c.unit.FromUI <- m
		}
	}
}
