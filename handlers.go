package main

import (
	"net/http"

	"github.com/gorilla/websocket"
)

func wsHandler(w http.ResponseWriter, r *http.Request) {
	ws, err := websocket.Upgrade(w, r, nil, 1024, 1024)
	if _, ok := err.(websocket.HandshakeError); ok {
		http.Error(w, "Not a websocket handshake", 400)
		return
	} else if err != nil {
		return
	}

	c := &uiConn{
		send: make(chan UIMsg, 1),
		ws:   ws}

	hub.uiReg <- c
	defer func() {
		hub.uiUnReg <- c
	}()
	go c.writer()
	c.reader()
}
