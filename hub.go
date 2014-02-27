package main

import (
	"encoding/json"

	"github.com/gorilla/websocket"
	"github.com/loggo/loggo"
)

var hubLogger = loggo.GetLogger("hub")

// Hub waits for webscoket-connections coming from Koha's user interface.
// For each websocket-connection it attempts to open a TCP-connection to a
// RFID-unit on the same IP-adress as the websocket. If successfull, a state-
// machine is started to handle all communications between the UI, SIP and the
// RFID-unit.
type Hub struct {
	// A map of connected UI connections:
	uiConnections map[*uiConn]bool
	// Register a new UI connection:
	uiReg chan *uiConn
	// Unregister a UI connection:
	uiUnReg chan *uiConn
	// Broadcast to all connected UIs (or optionally filtered by IP):
	broadcast chan encapsulatedUIMsg
}

// newHub creates and returns a new Hub instance.
func newHub() *Hub {
	return &Hub{
		uiConnections: make(map[*uiConn]bool),
		uiReg:         make(chan *uiConn),
		uiUnReg:       make(chan *uiConn),
		broadcast:     make(chan encapsulatedUIMsg),
	}
}

// run the Hub. Meant to be run in its own goroutine.
func (h *Hub) run() {
	for {
		select {
		case c := <-h.uiReg:
			h.uiConnections[c] = true
			hubLogger.Infof("WS   Connected %v", addr2IP(c.ws.RemoteAddr().String()))
		case c := <-h.uiUnReg:
			// TODO I shouldnt have to do this; but got panic because
			//      "close of closed channel" on some occations.
			if _, ok := h.uiConnections[c]; !ok {
				break
			}
			srv.stopChan <- addr2IP(c.ws.RemoteAddr().String())
			delete(h.uiConnections, c)
			close(c.send)
			hubLogger.Infof("WS   Disconnected")
		case msg := <-h.broadcast:
			hubLogger.Infof("-> UI %+v", msg)
			for c := range h.uiConnections {
				if (c.ipFilter == "") || (c.ipFilter == addr2IP(msg.IP)) {
					select {
					case c.send <- msg:
					default:
						srv.stopChan <- addr2IP(c.ws.RemoteAddr().String())
						close(c.send)
						delete(h.uiConnections, c)
					}
				}
			}
		}
	}
}

type uiConn struct {
	ws   *websocket.Conn
	send chan encapsulatedUIMsg
	// If ipFilter is an empty string, it means the subscriber wants all messages,
	// otherwise filter by IP:
	ipFilter string
}

func (c *uiConn) writer() {
	for message := range c.send {
		err := c.ws.WriteJSON(message.Msg)
		if err != nil {
			break
		}
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
		srv.fromUI <- encapsulatedUIMsg{
			IP:  addr2IP(c.ws.RemoteAddr().String()),
			Msg: m,
		}
	}
}
