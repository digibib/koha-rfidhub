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

var uiChan chan UIMsg

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
			println(err.Error())
			break
		}
		err = w.Flush()
		if err != nil {
			println(err.Error())
			break
		}
	}
}

func (d *dummyRFID) run() {
	ln, err := net.Listen("tcp", "127.0.0.1:"+cfg.TCPPort)
	if err != nil {
		println(err.Error())
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

func (a *dummyUIAgent) run(c chan UIMsg) {
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
}

func init() {
	loggo.ConfigureLoggers("<root>=INFO")
	//loggo.RemoveWriter("default")

	// setup & start the hub
	cfg = &config{TCPPort: "6007"}
	sipPool = NewSIPConnPool(0)
	uiChan = make(chan UIMsg)
	hub = newHub()

	go hub.run()
	http.HandleFunc("/ws", wsHandler)
	go http.ListenAndServe("127.0.0.1:8888", nil)
}

func TestMissingRFIDUnit(t *testing.T) {
	sipPool.initFn = FailingSIPResponse()
	sipPool.Init(1)
	a := newDummyUIAgent()
	ws, _, err := websocket.DefaultDialer.Dial("ws://127.0.0.1:8888/ws", nil)
	if err != nil {
		t.Fatal("Cannot get ws connection to 127.0.0.1:8888/ws")
	}
	a.c = ws
	go a.run(uiChan)

	uiMsg := <-uiChan
	want := UIMsg{Action: "CONNECT", RFIDError: true}
	if !reflect.DeepEqual(uiMsg, want) {
		t.Errorf("Got %+v; want %+v", uiMsg, want)
		t.Fatal("UI didn't get notified of failed RFID connect")
	}

	a.c.Close()
}

func TestRFIDUnitInitVersionFailure(t *testing.T) {
	sipPool.initFn = FailingSIPResponse()
	sipPool.Init(1)
	var d = newDummyRFID()
	go d.run()

	a := newDummyUIAgent()
	ws, _, err := websocket.DefaultDialer.Dial("ws://127.0.0.1:8888/ws", nil)
	if err != nil {
		t.Fatal("Cannot get ws connection to 127.0.0.1:8888/ws")
	}
	a.c = ws
	go a.run(uiChan)

	msg := <-d.incoming
	if string(msg) != "VER2.00\r" {
		t.Fatal("RFID-unit didn't get version init command")
	}

	d.outgoing <- []byte("NOK\r")

	uiMsg := <-uiChan
	want := UIMsg{Action: "CONNECT", RFIDError: true}
	if !reflect.DeepEqual(uiMsg, want) {
		t.Errorf("Got %+v; want %+v", uiMsg, want)
		t.Fatal("UI didn't get notified of failed RFID connect")
	}

	a.c.Close()
	d.c.Close()
	time.Sleep(100 * time.Millisecond)
}

func TestUnavailableSIPServer(t *testing.T) {
	sipPool.initFn = FailingSIPResponse()
	sipPool.Init(1)
	var d = newDummyRFID()
	go d.run()
	a := newDummyUIAgent()
	ws, _, err := websocket.DefaultDialer.Dial("ws://127.0.0.1:8888/ws", nil)
	if err != nil {
		t.Fatal("Cannot get ws connection to 127.0.0.1:8888/ws")
	}
	a.c = ws
	go a.run(uiChan)

	msg := <-d.incoming
	if string(msg) != "VER2.00\r" {
		t.Fatal("RFID-unit didn't get version init command")
	}
	d.outgoing <- []byte("OK\r")
	_ = <-uiChan
	err = a.c.WriteMessage(websocket.TextMessage, []byte(`{"Action":"CHECKIN"}`))
	if err != nil {
		t.Fatal("UI failed to send message over websokcet conn")
	}
	msg = <-d.incoming
	if string(msg) != "BEG\r" {
		t.Fatal("UI -> CHECKIN: RFID-unit didn't get instructed to start scanning")
	}
	d.outgoing <- []byte("OK\r")
	d.outgoing <- []byte("RDT1003010824124004:NO:02030000|1\r")

	uiMsg := <-uiChan
	want := UIMsg{Action: "CONNECT", SIPError: true}
	if !reflect.DeepEqual(uiMsg, want) {
		t.Errorf("Got %+v; want %+v", uiMsg, want)
		t.Fatal("UI didn't get notified of SIP error")
	}

	a.c.Close()
	d.c.Close()
}

func TestEmptySIPConnPool(t *testing.T) {
	sipPool.initFn = FailingSIPResponse()
	sipPool.Init(0)

	a := newDummyUIAgent()
	ws, _, err := websocket.DefaultDialer.Dial("ws://127.0.0.1:8888/ws", nil)
	if err != nil {
		t.Fatal("Cannot get ws connection to 127.0.0.1:8888/ws")
	}
	a.c = ws
	go a.run(uiChan)

	uiMsg := <-uiChan

	want := UIMsg{Action: "CONNECT", SIPError: true}
	if !reflect.DeepEqual(uiMsg, want) {
		t.Errorf("Got %+v; want %+v", uiMsg, want)
		t.Errorf("UI didn't get SIP error when SIP pool is empty")
	}
	a.c.Close()
}

func TestCheckins(t *testing.T) {
	sipPool.initFn = FailingSIPResponse()
	sipPool.Init(1)

	// Create & start the dummy RFID tcp server
	var d = newDummyRFID()
	go d.run()

	// Connect dummy UI agent
	a := newDummyUIAgent()
	ws, _, err := websocket.DefaultDialer.Dial("ws://127.0.0.1:8888/ws", nil)
	if err != nil {
		t.Fatal("Cannot get ws connection to 127.0.0.1:8888/ws")
	}
	a.c = ws
	go a.run(uiChan)

	// TESTS /////////////////////////////////////////////////////////////////

	msg := <-d.incoming
	if string(msg) != "VER2.00\r" {
		t.Fatal("RFID-unit didn't get version init command")
	}

	d.outgoing <- []byte("OK\r")

	// Verify that UI get's CONNECT message
	uiMsg := <-uiChan
	want := UIMsg{Action: "CONNECT"}
	if !reflect.DeepEqual(uiMsg, want) {
		t.Errorf("Got %+v; want %+v", uiMsg, want)
		t.Fatal("UI didn't get notified of succesfull rfid connect")
	}

	// Send "CHECKIN" message from UI and verify that the UI gets notified of
	// succesfull connect & RFID-unit that gets instructed to starts scanning for tags.
	err = a.c.WriteMessage(websocket.TextMessage, []byte(`{"Action":"CHECKIN"}`))
	if err != nil {
		t.Fatal("UI failed to send message over websokcet conn")
	}

	msg = <-d.incoming
	if string(msg) != "BEG\r" {
		t.Fatal("UI -> CHECKIN: RFID-unit didn't get instructed to start scanning")
	}

	// acknowledge BEG command
	d.outgoing <- []byte("OK\r")

	// Simulate found book on RFID-unit. Verify that it get's checked in through
	// SIP, the Alarm turned on, and that UI get's notified of the transaction
	sipPool.initFn = fakeSIPResponse("101YNN20140226    161239AO|AB03010824124004|AQfhol|AJHeavy metal in Baghdad|AA2|CS927.8|\r")
	sipPool.Init(1)
	d.outgoing <- []byte("RDT1003010824124004:NO:02030000|0\r")

	msg = <-d.incoming
	if string(msg) != "OK1\r" {
		t.Errorf("Alarm didnt get turn on after checkin")
	}
	d.outgoing <- []byte("OK\r")

	uiMsg = <-uiChan
	want = UIMsg{Action: "CHECKIN",
		Item: item{
			Label:   "Heavy metal in Baghdad",
			OK:      true,
			Barcode: "03010824124004",
			Date:    "26/02/2014",
		}}
	if !reflect.DeepEqual(uiMsg, want) {
		t.Errorf("Got %+v; want %+v", uiMsg, want)
		t.Fatal("UI didn't get the correct message after checkin")
	}

	// Simulate book on RFID-unit, but with missing tags. Verify that UI gets
	// notified with the books title, along with an error message
	sipPool.initFn = fakeSIPResponse("1803020120140226    203140AB03010824124004|AJHeavy metal in Baghdad|AQfhol|BGfhol|\r")
	sipPool.Init(1)
	d.outgoing <- []byte("RDT1003010824124004:NO:02030000|1\r")

	msg = <-d.incoming
	if string(msg) != "OK \r" {
		t.Errorf("Alarm was changed after unsuccessful checkin")
	}
	d.outgoing <- []byte("OK\r")

	uiMsg = <-uiChan
	want = UIMsg{Action: "CHECKIN",
		Item: item{
			Label:   "Heavy metal in Baghdad",
			Barcode: "03010824124004",
			OK:      false,
			Status:  "IKKE innlevert",
		}}
	if !reflect.DeepEqual(uiMsg, want) {
		t.Errorf("Got %+v; want %+v", uiMsg, want)
		t.Fatal("UI didn't get the correct message when item is missing tags")
	}

	// Verify that the RFID-unit gets END message when the corresponding
	// websocket connection is closed.
	a.c.Close()

	// TODO I'm closing connection before, is this necessary?
	// msg = <-d.incoming
	// if string(msg) != "END\r" {
	// 	t.Fatal("RFID-unit didn't get END message when UI connection was lost")
	// }

	// Disconnect RFIDUnit
	d.c.Close()
	time.Sleep(10 * time.Millisecond)
}

func TestCheckouts(t *testing.T) {
	sipPool.initFn = FailingSIPResponse()
	sipPool.Init(1)
	var d = newDummyRFID()
	go d.run()
	a := newDummyUIAgent()
	ws, _, err := websocket.DefaultDialer.Dial("ws://127.0.0.1:8888/ws", nil)
	if err != nil {
		t.Fatal("Cannot get ws connection to 127.0.0.1:8888/ws")
	}
	a.c = ws
	go a.run(uiChan)

	// TESTS /////////////////////////////////////////////////////////////////

	msg := <-d.incoming
	if string(msg) != "VER2.00\r" {
		t.Fatal("RFID-unit didn't get version init command")
	}
	d.outgoing <- []byte("OK\r")

	uiMsg := <-uiChan
	want := UIMsg{Action: "CONNECT"}
	if !reflect.DeepEqual(uiMsg, want) {
		t.Errorf("Got %+v; want %+v", uiMsg, want)
		t.Fatal("UI didn't get notified of succesfull rfid connect")
	}

	// Send "CHECKOUT" message from UI and verify that the UI gets notified of
	// succesfull connect & RFID-unit that gets instructed to starts scanning for tags.
	err = a.c.WriteMessage(websocket.TextMessage, []byte(`{"Action":"CHECKOUT", "Patron": "95"}`))
	if err != nil {
		t.Fatal("UI failed to send message over websokcet conn")
	}

	msg = <-d.incoming
	if string(msg) != "BEG\r" {
		t.Fatal("UI -> CHECKOUT: RFID-unit didn't get instructed to start scanning")
	}

	// acknowledge BEG command
	d.outgoing <- []byte("OK\r")

	// Simulate book on RFID-unit, but SIP show that book is allready checked out.
	// Verify that UI gets notified with the books title, along with an error message
	sipPool.initFn = fakeSIPResponse("120NUN20140303    102741AOHUTL|AA95|AB03011174511003|AJKrutt-Kim|AH|AFItem checked out to another patron|BLY|\r")
	sipPool.Init(1)
	d.outgoing <- []byte("RDT1003011174511003:NO:02030000|0\r")

	msg = <-d.incoming
	if string(msg) != "OK \r" {
		t.Errorf("Alarm was changed after unsuccessful checkout")
	}
	d.outgoing <- []byte("OK\r")

	uiMsg = <-uiChan
	want = UIMsg{Action: "CHECKOUT",
		Item: item{
			Label:   "Krutt-Kim",
			OK:      false,
			Barcode: "03011174511003",
			Status:  "Item checked out to another patron",
		}}
	if !reflect.DeepEqual(uiMsg, want) {
		t.Errorf("Got %+v; want %+v", uiMsg, want)
		t.Fatal("UI didn't get the correct message when item is allready checked out to another patron")
	}

	// Test successfull checkout
	sipPool.initFn = fakeSIPResponse("121NNY20140303    110236AOHUTL|AA95|AB03011063175001|AJCat's cradle|AH20140331    235900|\r")
	sipPool.Init(1)
	d.outgoing <- []byte("RDT1003011063175001:NO:02030000|0\r")

	msg = <-d.incoming
	if string(msg) != "OK0\r" {
		t.Errorf("Alarm was not turned off after successful checkout")
	}

	d.outgoing <- []byte("OK\r")

	uiMsg = <-uiChan
	want = UIMsg{Action: "CHECKOUT",
		Item: item{
			Label:   "Cat's cradle",
			OK:      true,
			Barcode: "03011063175001",
			Date:    "31/03/2014",
		}}
	if !reflect.DeepEqual(uiMsg, want) {
		t.Errorf("Got %+v; want %+v", uiMsg, want)
		t.Fatal("UI didn't get the correct message after succesfull checkout")
	}

	a.c.Close()
	d.c.Close()
}

// Test that rereading of items with missing tags doesn't trigger multiple SIP-calls
func TestBarcodesSession(t *testing.T) {
	sipPool.initFn = fakeSIPResponse("1803020120140226    203140AB03010824124004|AJHeavy metal in Baghdad|AQfhol|BGfhol|\r")
	sipPool.Init(1)
	var d = newDummyRFID()
	go d.run()
	a := newDummyUIAgent()
	ws, _, err := websocket.DefaultDialer.Dial("ws://127.0.0.1:8888/ws", nil)
	if err != nil {
		t.Fatal("Cannot get ws connection to 127.0.0.1:8888/ws")
	}
	a.c = ws
	go a.run(uiChan)

	msg := <-d.incoming
	if string(msg) != "VER2.00\r" {
		t.Fatal("RFID-unit didn't get version init command")
	}
	d.outgoing <- []byte("OK\r")
	_ = <-uiChan // CONNECT OK

	err = a.c.WriteMessage(websocket.TextMessage, []byte(`{"Action":"CHECKIN"}`))
	if err != nil {
		t.Fatal("UI failed to send message over websokcet conn")
	}

	_ = <-d.incoming
	d.outgoing <- []byte("OK\r")

	d.outgoing <- []byte("RDT1003010824124004:NO:02030000|1\r")
	msg = <-d.incoming
	d.outgoing <- []byte("OK\r")

	_ = <-uiChan
	sipPool.initFn = FailingSIPResponse()
	sipPool.Init(1)
	d.outgoing <- []byte("RDT1003010824124004:NO:02030000|1\r")
	msg = <-d.incoming
	d.outgoing <- []byte("OK\r")

	uiMsg := <-uiChan

	if uiMsg.SIPError {
		t.Fatalf("Rereading of failed tags triggered multiple SIP-calls")
	}
	d.c.Close()

}

// Verify that if a second websocket connection is opened from the same IP,
// the first connection is closed.
// func TestDuplicateClientConnections(t *testing.T) {
// 	sipPool.initFn = FailingSIPResponse()
// 	sipPool.Init(0)

// 	ws, _, err := websocket.DefaultDialer.Dial("ws://127.0.0.1:8888/ws", nil)
// 	if err != nil {
// 		t.Fatal("Cannot get ws connection to 127.0.0.1:8888/ws")
// 	}

// 	time.Sleep(100 * time.Millisecond)

// 	_, _, err = websocket.DefaultDialer.Dial("ws://127.0.0.1:8888/ws", nil)
// 	if err != nil {
// 		t.Fatal("Cannot get ws connection to 127.0.0.1:8888/ws")
// 	}

// 	time.Sleep(100 * time.Millisecond)

// 	// Try writing to the first connection; it should be closed:
// 	err = ws.WriteMessage(websocket.TextMessage, []byte(`{"Action":"CONNECT"}`))
// 	if err == nil {
// 		t.Error("Two ws connections from the same IP should not be allowed")
// 	}
// 	time.Sleep(100 * time.Millisecond)

// }
