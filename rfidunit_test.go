package main

import (
	"bufio"
	"net"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/loggo/loggo"
)

// TODO when done testing:
// func init() {
// 	loggo.RemoveWriter("default")
// }

func init() {
	loggo.ConfigureLoggers("<root>=INFO")
}

type dummyRFID struct {
	c        net.Conn
	incoming chan []byte
}

func (d *dummyRFID) reader() {
	r := bufio.NewReader(d.c)
	for {
		msg, err := r.ReadBytes('\r')
		if err != nil {
			close(d.incoming)
			break
		}
		d.incoming <- msg
	}
}

func newDummyRFID() *dummyRFID {
	return &dummyRFID{
		incoming: make(chan []byte),
	}
}

type dummyUIAgent struct {
	c *websocket.Conn
}

func newDummyUIAgent() *dummyUIAgent {
	return &dummyUIAgent{}
}

func TestRFIDUnitStateMachine(t *testing.T) {

	// SETUP//////////////////////////////////////////////////////////////////
	sipPool = NewSIPConnPool(0)

	// Start tcp server
	cfg := &config{TCPPort: "7777"}
	srv = newTCPServer(cfg)
	uiChan := make(chan encapsulatedUIMsg, 10)
	srv.broadcast = uiChan
	go srv.run()

	// Start http server and websocket hub
	uiHub = newHub()
	go uiHub.run()
	http.HandleFunc("/ws", wsHandler)
	go http.ListenAndServe("localhost:8888", nil)

	// Wait til all services are running
	time.Sleep(100 * time.Millisecond)

	// Connect with simluated rfid-unit
	d := newDummyRFID()
	c, err := net.Dial("tcp", "localhost:7777")
	if err != nil {
		t.Fatal("Cannot connect to TCP server, localhost:7777")
	}
	d.c = c
	go d.reader()

	// Connect dummy UI agent
	a := newDummyUIAgent()
	ws, _, err := websocket.DefaultDialer.Dial("ws://localhost:8888/ws", nil)
	if err != nil {
		t.Fatal("Cannot get ws connection to localhost:8888/ws")
	}
	a.c = ws

	if addr2IP(d.c.RemoteAddr().String()) != addr2IP(a.c.RemoteAddr().String()) {
		t.Fatal("RFID-unit and websocket connection has different IPs")
	}

	// TESTS /////////////////////////////////////////////////////////////////

	// Send "CHECKIN" message from UI and verify that the RFID-unit gets
	// instructed to starts scanning for tags.
	err = a.c.WriteMessage(websocket.TextMessage, []byte(`{"Action":"CHECKIN"}`))
	if err != nil {
		t.Fatal("UI failed to send message over websokcet conn")
	}

	msg := <-d.incoming
	if string(msg) != "BEG\r" {
		t.Fatal("UI -> CHECKIN: RFID-unit didn't get instructed to start scanning")
	}

	// Simulate found book on RFID-unit. Verify that it get's checked in through
	// SIP and that UI get's notified of the transaction
	sipPool.Init(1, fakeSIPResponse("101YNN20140226    161239AO|AB03010824124004|AQfhol|AJHeavy metal in Baghdad|AA2|CS927.8|\r"))
	d.c.Write([]byte("RDT1003010824124004:NO:02030000|0\r"))
	uiMsg := <-uiChan

	want := UIMsg{Action: "CHECKIN",
		Item: item{
			Label:  "Heavy metal in Baghdad",
			OK:     true,
			Status: "registrert innlevert 26/02/2014",
		}}
	if !reflect.DeepEqual(uiMsg.Msg, want) {
		t.Errorf("Got %+v; want %+v", uiMsg.Msg, want)
		t.Fatal("UI didn't get the correct message after checkin")
	}

	//

	// // Verify that the RFID-unit gets END message when the corresponding
	// // websocket connection is closed.
	// a.c.Close()

	// msg = <-d.incoming
	// if string(msg) != "END\r" {
	// 	t.Fatal("RFID-unit didn't get END message when UI connection was lost")
	// }
}
