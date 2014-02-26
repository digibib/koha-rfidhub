package main

import (
	"encoding/json"

	"github.com/gorilla/websocket"
	"github.com/loggo/loggo"
)

var wsLogger = loggo.GetLogger("ws")

type uiConn struct {
	ws   *websocket.Conn
	send chan encapsulatedUIMsg
	// If ipFilter is an empty string, it means the subscriber wants all messages,
	// otherwise filter by IP:
	ipFilter string
}

func (c *uiConn) writer() {
	for message := range c.send {
		err := c.ws.WriteJSON(message)
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

type wsHub struct {
	connections map[*uiConn]bool
	uiReg       chan *uiConn // Register connection
	uiUnReg     chan *uiConn // Unregister connection

	broadcast chan encapsulatedUIMsg // Broadcast to all connected UIs
}

func newHub() *wsHub {
	return &wsHub{
		connections: make(map[*uiConn]bool),
		uiReg:       make(chan *uiConn),
		uiUnReg:     make(chan *uiConn),
		broadcast:   make(chan encapsulatedUIMsg),
	}
}

func (h *wsHub) run() {
	for {
		select {
		case c := <-h.uiReg:
			h.connections[c] = true
			wsLogger.Infof("WS   Connected")
		case c := <-h.uiUnReg:
			// TODO I shouldnt have to do this; but got panic because
			//      "close of closed channel" on some occations.
			if _, ok := h.connections[c]; !ok {
				break
			}
			srv.stopChan <- addr2IP(c.ws.RemoteAddr().String())
			delete(h.connections, c)
			close(c.send)
			wsLogger.Infof("WS   Disconnected")
		case msg := <-h.broadcast:
			wsLogger.Infof("-> UI %+v", msg)
			for c := range h.connections {
				if (c.ipFilter == "") || (c.ipFilter == addr2IP(msg.IP)) {
					select {
					case c.send <- msg:
					default:
						srv.stopChan <- addr2IP(c.ws.RemoteAddr().String())
						close(c.send)
						delete(h.connections, c)
					}
				}
			}
		}
	}
}
