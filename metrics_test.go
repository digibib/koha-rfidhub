package main

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/gorilla/websocket"
)

func TestStatusEndpoint(t *testing.T) {
	r, err := http.Get("http://localhost:8888/.status")
	if err != nil {
		t.Fatal(err)
	}

	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		t.Fatal(err)
	}
	status := exportMetrics{}
	err = json.Unmarshal(b, &status)
	if err != nil {
		t.Fatal(err)
	}

	if status.ClientsConnected != 0 {
		t.Errorf("status.ClientsConnected => %v, expected 0", status.ClientsConnected)
	}

	a := newDummyUIAgent()
	ws, _, err := websocket.DefaultDialer.Dial("ws://127.0.0.1:8888/ws", nil)
	if err != nil {
		t.Fatal("Cannot get ws connection to 127.0.0.1:8888/ws")
	}
	a.c = ws
	go a.run(uiChan)

	r.Body.Close()
	r, err = http.Get("http://localhost:8888/.status")
	if err != nil {
		t.Fatal(err)
	}

	b, err = ioutil.ReadAll(r.Body)
	if err != nil {
		t.Fatal(err)
	}

	err = json.Unmarshal(b, &status)
	if err != nil {
		t.Fatal(err)
	}

	if status.ClientsConnected != 1 {
		t.Errorf("status.ClientsConnected => %v, expected 1", status.ClientsConnected)
	}

	<-uiChan

	a.c.Close()
}
