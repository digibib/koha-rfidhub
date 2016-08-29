package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStatusEndpoint(t *testing.T) {
	// Setup: ->

	uiChan := make(chan UIMsg)
	sipSrv := newSIPTestServer()
	defer sipSrv.Close()

	srv := httptest.NewServer(nil)
	defer srv.Close()

	hub = newHub(config{
		HTTPPort:          port(srv.URL),
		SIPServer:         sipSrv.Addr(),
		TCPPort:           "12346", // not listening
		NumSIPConnections: 1,
	})
	go hub.run()
	defer hub.Close()

	// <- end setup

	r, err := http.Get(fmt.Sprintf("http://localhost:%s/.status", port(srv.URL)))
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

	a := newDummyUIAgent(uiChan, port(srv.URL))
	defer a.c.Close()

	r.Body.Close()
	r, err = http.Get(fmt.Sprintf("http://localhost:%s/.status", port(srv.URL)))

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
