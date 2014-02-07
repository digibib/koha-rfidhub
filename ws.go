package main

import (
	"github.com/gorilla/websocket"
	"github.com/loggo/loggo"
)

var wsLogger = loggo.GetLogger("ws")

type uiConn struct {
	ws   *websocket.Conn
	send chan UIMessage
}

func (c *uiConn) writer() {
	for message := range c.send {
		// TODO could filter to recipients here, but not good architecture
		// log.Println("DEBUG Same IP:", sameIP(addr2IP(c.ws.RemoteAddr().String()), addr2IP(message.ID)))
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
		srv.incoming <- msg
	}
}

type wsHub struct {
	connections map[*uiConn]bool
	uiReg       chan *uiConn // Register connection
	uiUnReg     chan *uiConn // Unregister connection

	broadcast chan UIMessage // Broadcast to all connected UIs
}

func newHub() *wsHub {
	return &wsHub{
		connections: make(map[*uiConn]bool),
		uiReg:       make(chan *uiConn),
		uiUnReg:     make(chan *uiConn),
		broadcast:   make(chan UIMessage),
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
			delete(h.connections, c)
			close(c.send)
			wsLogger.Infof("WS   Disconnected")
		case msg := <-h.broadcast:
			wsLogger.Infof("-> UI %+v", msg)
			for c := range h.connections {
				select {
				case c.send <- msg:
				default:
					close(c.send)
					delete(h.connections, c)
				}
			}
		}
	}
}
