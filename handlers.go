package main

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/websocket"
)

func statusHandler(w http.ResponseWriter, r *http.Request) {
	b, err := json.Marshal(status.Export())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(b)
}

func wsHandler(w http.ResponseWriter, r *http.Request) {
	ws, err := websocket.Upgrade(w, r, nil, 1024, 1024)
	if _, ok := err.(websocket.HandshakeError); ok {
		http.Error(w, "Not a websocket handshake", 400)
		return
	} else if err != nil {
		return
	}

	c := &uiConn{
		send: make(chan UIMsg, 10),
		ws:   ws}

	hub.uiReg <- c
	defer func() {
		hub.uiUnReg <- c
	}()

	// Count as connected
	status.ClientsConnected.Inc(1)

	go c.writer()
	c.reader()

	// Count as disconnected
	status.ClientsConnected.Dec(1)
}
