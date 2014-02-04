package main

import (
	"log"

	"github.com/gorilla/websocket"
)

type uiConn struct {
	ws   *websocket.Conn
	send chan UIMessage
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
		_, _, err := c.ws.ReadMessage()
		if err != nil {
			break
		}
	}
}

type wsHub struct {
	connections map[*uiConn]bool
	uiReg       chan *uiConn // Register connection
	uiUnReg     chan *uiConn // Unregister connection

	broadcast chan UIMessage // Broadcast to all connected UIs
}

func NewHub() *wsHub {
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
			log.Println("WS   Connected")
		case c := <-h.uiUnReg:
			delete(h.connections, c)
			close(c.send)
			log.Println("WS   Disconnected")
		case msg := <-h.broadcast:
			log.Printf("-> UI %+v", msg)
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
