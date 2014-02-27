package main

import (
	"bufio"
	"encoding/json"
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
	outgoing chan []byte
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

func (d *dummyRFID) writer() {
	w := bufio.NewWriter(d.c)
	for msg := range d.outgoing {
		_, err := w.Write(msg)
		if err != nil {
			panic(err)
			break
		}
		err = w.Flush()
		if err != nil {
			panic(err)
			break
		}
	}
}

func newDummyRFID() *dummyRFID {
	return &dummyRFID{
		incoming: make(chan []byte),
		outgoing: make(chan []byte),
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
	cfg = &config{TCPPort: "6005"}
	sipPool = NewSIPConnPool(0)
	uiChan := make(chan UIMsg)
	hub = newHub()

	var d = newDummyRFID()
	// Start the dummy RFID tcp server
	go func(d *dummyRFID) {
		ln, err := net.Listen("tcp", "127.0.0.1:"+cfg.TCPPort)
		if err != nil {
			panic("Cannot start dummy RFID TCP-server")
		}
		defer ln.Close()
		c, err := ln.Accept()
		if err != nil {
			println(err.Error())
			panic("cannot accept tcp connection")
		}
		d.c = c
		go d.writer()
		d.reader()
	}(d)

	time.Sleep(10 * time.Millisecond)

	// Start http server and websocket hub
	go hub.run()
	http.HandleFunc("/ws", wsHandler)
	go http.ListenAndServe("127.0.0.1:8888", nil)

	time.Sleep(100 * time.Millisecond)

	// Connect dummy UI agent
	a := newDummyUIAgent()
	ws, _, err := websocket.DefaultDialer.Dial("ws://127.0.0.1:8888/ws", nil)
	if err != nil {
		t.Fatal("Cannot get ws connection to 127.0.0.1:8888/ws")
	}
	a.c = ws
	go func(c chan UIMsg) {
		for {
			_, msg, err := a.c.ReadMessage()
			if err != nil {
				break
			}
			var m UIMsg
			err = json.Unmarshal(msg, &m)
			if err != nil {
				break
			}
			c <- m
		}
	}(uiChan)

	time.Sleep(100 * time.Millisecond)

	// TESTS /////////////////////////////////////////////////////////////////

	// Send "CHECKIN" message from UI and verify that the UI gets notified of
	// succesfull connect & RFID-unit that gets instructed to starts scanning for tags.
	err = a.c.WriteMessage(websocket.TextMessage, []byte(`{"Action":"CHECKIN"}`))
	if err != nil {
		t.Fatal("UI failed to send message over websokcet conn")
	}

	uiMsg := <-uiChan
	want := UIMsg{Action: "CONNECT"}
	if !reflect.DeepEqual(uiMsg, want) {
		t.Errorf("Got %+v; want %+v", uiMsg, want)
		t.Fatal("UI didn't get notified of succesfull rfid connect")
	}

	msg := <-d.incoming
	if string(msg) != "BEG\r" {
		t.Fatal("UI -> CHECKIN: RFID-unit didn't get instructed to start scanning")
	}

	// Simulate found book on RFID-unit. Verify that it get's checked in through
	// SIP, the Alarm turned on, and that UI get's notified of the transaction
	sipPool.Init(1, fakeSIPResponse("101YNN20140226    161239AO|AB03010824124004|AQfhol|AJHeavy metal in Baghdad|AA2|CS927.8|\r"))
	d.outgoing <- []byte("RDT1003010824124004:NO:02030000|0\r")

	msg = <-d.incoming
	if string(msg) != "OK1\r" {
		t.Errorf("Alarm didnt get turn on after checkin")
	}
	d.outgoing <- []byte("OK\r")

	println("blocking here:")
	uiMsg = <-uiChan
	want = UIMsg{Action: "CHECKIN",
		Item: item{
			Label:  "Heavy metal in Baghdad",
			OK:     true,
			Status: "registrert innlevert 26/02/2014",
		}}
	if !reflect.DeepEqual(uiMsg, want) {
		t.Errorf("Got %+v; want %+v", uiMsg, want)
		t.Fatal("UI didn't get the correct message after checkin")
	}

	// Simulate book on RFID-unit, but with missing tags. Verify that UI gets
	// notified with the books title, along with an error message
	sipPool.Init(1, fakeSIPResponse("1803020120140226    203140AB03010824124004|AJHeavy metal in Baghdad|AQfhol|BGfhol|\r"))
	d.c.Write([]byte("RDT1003010824124004:NO:02030000|1\r"))

	msg = <-d.incoming
	if string(msg) != "OK\r" {
		t.Errorf("Alarm was changed after unsuccessful checkin")
	}

	uiMsg = <-uiChan
	want = UIMsg{Action: "CHECKIN",
		Item: item{
			Label:  "Heavy metal in Baghdad",
			OK:     false,
			Status: "IKKE innlevert; mangler brikke!",
		}}
	if !reflect.DeepEqual(uiMsg, want) {
		t.Errorf("Got %+v; want %+v", uiMsg, want)
		t.Fatal("UI didn't get the correct message when item is missing tags")
	}

	// Test successfull checkout
	// TODO

	// Test unsucsessfull checkout
	// TODO

	// Verify that the RFID-unit gets END message when the corresponding
	// websocket connection is closed.
	a.c.Close()

	msg = <-d.incoming
	if string(msg) != "END\r" {
		t.Fatal("RFID-unit didn't get END message when UI connection was lost")
	}

	// Disconnect RFIDUnit
	d.c.Close()
	time.Sleep(10 * time.Millisecond)
	// TODO verify what?
}
