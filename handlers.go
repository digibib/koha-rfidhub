package main

import (
	"net/http"

	"github.com/gorilla/websocket"
)

func connectionsToUnits(conns map[string]*RFIDUnit) []string {
	var res []string
	for c := range conns {
		res = append(res, c)
	}
	return res
}

func testHandler(w http.ResponseWriter, r *http.Request) {
	data := struct {
		Host  string
		Units []string
	}{
		r.Host,
		connectionsToUnits(srv.connections),
	}
	err := templates.ExecuteTemplate(w, "index.html", data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func wsHandler(w http.ResponseWriter, r *http.Request) {
	ws, err := websocket.Upgrade(w, r, nil, 1024, 1024)
	if _, ok := err.(websocket.HandshakeError); ok {
		http.Error(w, "Not a websocket handshake", 400)
		return
	} else if err != nil {
		return
	}

	c := &uiConn{send: make(chan UIMessage), ws: ws}
	uiHub.uiReg <- c
	defer func() {
		uiHub.uiUnReg <- c
	}()
	go c.writer()
	c.reader()
}
