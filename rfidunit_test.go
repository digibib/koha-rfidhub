package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

type dummyRFID struct {
	mu       sync.Mutex
	ln       net.Listener
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
	var err error
	d.mu.Lock()
	d.ln, err = net.Listen("tcp", ":0")
	d.mu.Unlock()
	if err != nil {
		println(err.Error())
		panic("Cannot start dummy RFID TCP-server")
	}
	defer d.ln.Close()
	c, err := d.ln.Accept()
	if err != nil {
		println(err.Error())
		return
	}
	d.c = c
	go d.writer()
	d.reader()
}

func (d *dummyRFID) addr() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return "http://" + d.ln.Addr().String()
}

func (d *dummyRFID) Close() {
	if d.c != nil {
		d.c.Close()
	}
	close(d.outgoing)
	if d.ln != nil {
		d.ln.Close()
	}
}

func newDummyRFIDReader() *dummyRFID {
	d := dummyRFID{
		incoming: make(chan []byte),
		outgoing: make(chan []byte),
	}
	go d.run()
	return &d
}

type dummyUIAgent struct {
	c   *websocket.Conn
	msg chan UIMsg
}

func newDummyUIAgent(msg chan UIMsg, port string) *dummyUIAgent {
	d := dummyUIAgent{
		msg: msg,
	}
	ws, _, err := websocket.DefaultDialer.Dial(fmt.Sprintf("ws://localhost:%s/ws", port), nil)
	if err != nil {
		panic(fmt.Sprintf("Cannot connect to ws://localhost:%s/ws: %v", port, err.Error()))
	}
	d.c = ws
	go d.run()
	return &d
}

func (a *dummyUIAgent) run() {
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
		a.msg <- m
	}
}

func port(s string) string {
	return s[strings.LastIndex(s, ":")+1:]
}

func TestMissingRFIDUnit(t *testing.T) {
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

	a := newDummyUIAgent(uiChan, port(srv.URL))
	defer a.c.Close()

	// <- end setup

	uiMsg := <-uiChan
	want := UIMsg{Action: "CONNECT", RFIDError: true}
	if !reflect.DeepEqual(uiMsg, want) {
		t.Errorf("Got %+v; want %+v", uiMsg, want)
		t.Fatal("UI didn't get notified of failed RFID connect")
	}

}

func TestRFIDUnitInitVersionFailure(t *testing.T) {
	// Setup: ->

	uiChan := make(chan UIMsg)
	sipSrv := newSIPTestServer()
	defer sipSrv.Close()

	srv := httptest.NewServer(nil)
	defer srv.Close()

	d := newDummyRFIDReader()
	defer d.Close()

	time.Sleep(50) // make sure rfidreader has got designated a port and is listening

	hub = newHub(config{
		HTTPPort:          port(srv.URL),
		SIPServer:         sipSrv.Addr(),
		TCPPort:           port(d.addr()),
		NumSIPConnections: 1,
	})
	go hub.run()
	defer hub.Close()

	a := newDummyUIAgent(uiChan, port(srv.URL))
	defer a.c.Close()

	// <- end setup

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
}

func TestUnavailableSIPServer(t *testing.T) {
	// Setup: ->

	uiChan := make(chan UIMsg)
	sipSrv := newSIPTestServer().Failing()
	defer sipSrv.Close()

	srv := httptest.NewServer(nil)
	defer srv.Close()

	d := newDummyRFIDReader()
	defer d.Close()

	time.Sleep(50) // make sure rfidreader has got designated a port and is listening

	hub = newHub(config{
		HTTPPort:          port(srv.URL),
		SIPServer:         sipSrv.Addr(),
		TCPPort:           port(d.addr()),
		NumSIPConnections: 1,
	})
	go hub.run()
	defer hub.Close()

	a := newDummyUIAgent(uiChan, port(srv.URL))
	defer a.c.Close()

	// <- end setup

	msg := <-d.incoming
	if string(msg) != "VER2.00\r" {
		t.Fatal("RFID-unit didn't get version init command")
	}
	d.outgoing <- []byte("OK\r")
	<-uiChan
	if err := a.c.WriteMessage(websocket.TextMessage, []byte(`{"Action":"CHECKIN"}`)); err != nil {
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

}

func TestCheckins(t *testing.T) {
	// Setup: ->

	uiChan := make(chan UIMsg)
	sipSrv := newSIPTestServer()
	defer sipSrv.Close()

	srv := httptest.NewServer(nil)
	defer srv.Close()

	d := newDummyRFIDReader()
	defer d.Close()

	time.Sleep(50) // make sure rfidreader has got designated a port and is listening

	hub = newHub(config{
		HTTPPort:          port(srv.URL),
		SIPServer:         sipSrv.Addr(),
		TCPPort:           port(d.addr()),
		NumSIPConnections: 1,
	})
	go hub.run()
	defer hub.Close()

	a := newDummyUIAgent(uiChan, port(srv.URL))
	defer a.c.Close()

	// <- end setup

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
	err := a.c.WriteMessage(websocket.TextMessage, []byte(`{"Action":"CHECKIN","Branch":"fmaj"}`))
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
	sipSrv.Respond("101YNN20140226    161239AO|AB03010824124004|AQfhol|AJHeavy metal in Baghdad|CTfbol|AA2|CS927.8|\r")
	d.outgoing <- []byte("RDT1003010824124004:NO:02030000|0\r")

	msg = <-d.incoming
	if string(msg) != "OK1\r" {
		t.Errorf("Checkin: RFID reader didn't get instructed to turn on alarm")
	}
	// simulate failed alarm command
	d.outgoing <- []byte("NOK\r")

	uiMsg = <-uiChan
	want = UIMsg{Action: "CHECKIN",
		Item: item{
			Label:         "Heavy metal in Baghdad",
			Barcode:       "03010824124004",
			Date:          "26/02/2014",
			AlarmOnFailed: true,
			Transfer:      "fbol",
			Status:        "Feil: fikk ikke skrudd på alarm.",
		}}
	if !reflect.DeepEqual(uiMsg, want) {
		t.Errorf("Got %+v; want %+v", uiMsg, want)
		t.Fatal("UI didn't get the correct message after checkin")
	}

	// retry alarm on
	err = a.c.WriteMessage(websocket.TextMessage, []byte(`{"Action":"RETRY-ALARM-ON"}`))
	if err != nil {
		t.Fatal("UI failed to send message over websokcet conn")
	}

	msg = <-d.incoming
	if string(msg) != "ACT1003010824124004:NO:02030000\r" {
		t.Fatal("UI -> RETRY-ALARM-ON didn't trigger the right RFID command")
	}

	d.outgoing <- []byte("OK\r")

	uiMsg = <-uiChan
	want = UIMsg{Action: "CHECKIN",
		Item: item{
			Label:   "Heavy metal in Baghdad",
			Barcode: "03010824124004",
			Date:    "26/02/2014",
		}}
	if !reflect.DeepEqual(uiMsg, want) {
		t.Errorf("Got %+v; want %+v", uiMsg, want)
		t.Fatal("UI didn't get the correct message after checkin retry alarm on")
	}

	// Simulate barcode not in our db

	sipSrv.Respond("100NUY20140128    114702AO|AB1234|CV99|AFItem not checked out|\r")
	d.outgoing <- []byte("RDT1234:NO:02030000|0\r")

	msg = <-d.incoming
	if string(msg) != "OK \r" {
		t.Errorf("Alarm was changed after unsuccessful checkin")
	}

	d.outgoing <- []byte("OK\r")

	uiMsg = <-uiChan
	want = UIMsg{Action: "CHECKIN",
		Item: item{
			Barcode:           "1234",
			TransactionFailed: true,
			Unknown:           true,
			Status:            "eksemplaret finnes ikke i basen",
		}}
	if !reflect.DeepEqual(uiMsg, want) {
		t.Errorf("Got %+v; want %+v", uiMsg, want)
	}

	// Simulate book on RFID-unit, but with missing tags. Verify that UI gets
	// notified with the books title, along with an error message
	sipSrv.Respond("1803020120140226    203140AB03010824124004|AO|AJHeavy metal in Baghdad|AQfhol|BGfhol|\r")
	d.outgoing <- []byte("RDT1003010824124004:NO:02030000|1\r")

	msg = <-d.incoming
	if string(msg) != "OK \r" {
		t.Error("Alarm was changed after unsuccessful checkin")
	}
	d.outgoing <- []byte("OK\r")

	uiMsg = <-uiChan
	want = UIMsg{Action: "CHECKIN",
		Item: item{
			Label:             "Heavy metal in Baghdad",
			Barcode:           "03010824124004",
			TransactionFailed: true,
		}}
	if !reflect.DeepEqual(uiMsg, want) {
		t.Errorf("Got %+v; want %+v", uiMsg, want)
		t.Fatal("UI didn't get the correct message when item is missing tags")
	}

	// Verify that the RFID-unit gets END message when the corresponding
	// websocket connection is closed.

	// TODO I'm closing connection before, is this necessary?
	// msg = <-d.incoming
	// if string(msg) != "END\r" {
	// 	t.Fatal("RFID-unit didn't get END message when UI connection was lost")
	// }

}

func TestCheckouts(t *testing.T) {

	// setup ->

	uiChan := make(chan UIMsg)
	sipSrv := newSIPTestServer()
	defer sipSrv.Close()

	srv := httptest.NewServer(nil)
	defer srv.Close()

	d := newDummyRFIDReader()
	defer d.Close()

	time.Sleep(50) // make sure rfidreader has got designated a port and is listening

	hub = newHub(config{
		HTTPPort:          port(srv.URL),
		SIPServer:         sipSrv.Addr(),
		TCPPort:           port(d.addr()),
		NumSIPConnections: 1,
	})
	go hub.run()
	defer hub.Close()

	a := newDummyUIAgent(uiChan, port(srv.URL))
	defer a.c.Close()

	// <- end setup

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
	err := a.c.WriteMessage(websocket.TextMessage, []byte(`{"Action":"CHECKOUT", "Patron": "95", "Branch":"hutl"}`))
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
	sipSrv.Respond("120NUN20140303    102741AOHUTL|AA95|AB03011174511003|AJKrutt-Kim|AH|AFItem checked out to another patron|BLY|\r")
	d.outgoing <- []byte("RDT1003011174511003:NO:02030000|0\r")

	msg = <-d.incoming
	if string(msg) != "OK \r" {
		t.Errorf("Alarm was changed after unsuccessful checkout")
	}

	d.outgoing <- []byte("OK\r")

	uiMsg = <-uiChan
	want = UIMsg{Action: "CHECKOUT",
		Item: item{
			Label:             "Krutt-Kim",
			Barcode:           "03011174511003",
			TransactionFailed: true,
			Status:            "Item checked out to another patron",
		}}
	if !reflect.DeepEqual(uiMsg, want) {
		t.Errorf("Got %+v; want %+v", uiMsg, want)
		t.Fatal("UI didn't get the correct message when item is allready checked out to another patron")
	}

	// Test successfull checkout
	sipSrv.Respond("121NNY20140303    110236AOHUTL|AA95|AB03011063175001|AJCat's cradle|AH20140331    235900|\r")
	d.outgoing <- []byte("RDT1003011063175001:NO:02030000|0\r")

	msg = <-d.incoming
	if string(msg) != "OK0\r" {
		t.Errorf("Alarm was not turned off after successful checkout")
	}

	// simulate failed alarm
	d.outgoing <- []byte("NOK\r")

	uiMsg = <-uiChan
	want = UIMsg{Action: "CHECKOUT",
		Item: item{
			Label:          "Cat's cradle",
			Barcode:        "03011063175001",
			Date:           "03/03/2014",
			AlarmOffFailed: true,
			Status:         "Feil: fikk ikke skrudd av alarm.",
		}}
	if !reflect.DeepEqual(uiMsg, want) {
		t.Errorf("Got %+v; want %+v", uiMsg, want)
		t.Fatal("UI didn't get the correct message after succesfull checkout")
	}

	// retry alarm off
	err = a.c.WriteMessage(websocket.TextMessage, []byte(`{"Action":"RETRY-ALARM-OFF"}`))
	if err != nil {
		t.Fatal("UI failed to send message over websokcet conn")
	}

	msg = <-d.incoming
	if string(msg) != "DAC1003011063175001:NO:02030000\r" {
		t.Errorf("Got %q, Want DAC1003011063175001:NO:02030000", msg)
		t.Fatal("UI -> RETRY-ALARM-ON didn't trigger the right RFID command")
	}

	d.outgoing <- []byte("OK\r")

	uiMsg = <-uiChan
	want = UIMsg{Action: "CHECKOUT",
		Item: item{
			Label:   "Cat's cradle",
			Barcode: "03011063175001",
			Date:    "03/03/2014",
		}}
	if !reflect.DeepEqual(uiMsg, want) {
		t.Errorf("Got %+v; want %+v", uiMsg, want)
		t.Fatal("UI didn't get the correct message after succesfull checkout")
	}

}

// Test that rereading of items with missing tags doesn't trigger multiple SIP-calls
func TestBarcodesSession(t *testing.T) {
	// setup ->

	uiChan := make(chan UIMsg)
	sipSrv := newSIPTestServer()
	defer sipSrv.Close()

	srv := httptest.NewServer(nil)
	defer srv.Close()

	d := newDummyRFIDReader()
	defer d.Close()

	time.Sleep(50) // make sure rfidreader has got designated a port and is listening

	hub = newHub(config{
		HTTPPort:          port(srv.URL),
		SIPServer:         sipSrv.Addr(),
		TCPPort:           port(d.addr()),
		NumSIPConnections: 1,
	})
	go hub.run()
	defer hub.Close()

	a := newDummyUIAgent(uiChan, port(srv.URL))
	defer a.c.Close()

	// <- end setup

	msg := <-d.incoming
	if string(msg) != "VER2.00\r" {
		t.Fatal("RFID-unit didn't get version init command")
	}
	d.outgoing <- []byte("OK\r")
	<-uiChan // CONNECT OK

	err := a.c.WriteMessage(websocket.TextMessage, []byte(`{"Action":"CHECKIN"}`))
	if err != nil {
		t.Fatal("UI failed to send message over websokcet conn")
	}

	<-d.incoming
	d.outgoing <- []byte("OK\r")

	sipSrv.Respond("1803020120140226    203140AB03010824124004|AJHeavy metal in Baghdad|AQfhol|BGfhol|\r")
	d.outgoing <- []byte("RDT1003010824124004:NO:02030000|1\r")
	<-d.incoming
	d.outgoing <- []byte("OK\r")

	<-uiChan
	d.outgoing <- []byte("RDT1003010824124004:NO:02030000|1\r")
	<-d.incoming
	d.outgoing <- []byte("OK\r")

	uiMsg := <-uiChan

	if uiMsg.SIPError {
		t.Fatalf("Rereading of failed tags triggered multiple SIP-calls")
	}
}

func TestWriteLogic(t *testing.T) {
	// setup ->

	uiChan := make(chan UIMsg)
	sipSrv := newSIPTestServer()
	defer sipSrv.Close()

	srv := httptest.NewServer(nil)
	defer srv.Close()

	d := newDummyRFIDReader()
	defer d.Close()

	time.Sleep(50) // make sure rfidreader has got designated a port and is listening

	hub = newHub(config{
		HTTPPort:          port(srv.URL),
		SIPServer:         sipSrv.Addr(),
		TCPPort:           port(d.addr()),
		NumSIPConnections: 1,
	})
	go hub.run()
	defer hub.Close()

	a := newDummyUIAgent(uiChan, port(srv.URL))
	defer a.c.Close()

	// <- end setup

	sipSrv.Respond("1803020120140226    203140AB03010824124004|AJHeavy metal in Baghdad|AQfhol|BGfhol|\r")

	msg := <-d.incoming
	if string(msg) != "VER2.00\r" {
		t.Fatal("RFID-unit didn't get version init command")
	}
	d.outgoing <- []byte("OK\r")
	<-uiChan // CONNECT OK

	err := a.c.WriteMessage(websocket.TextMessage,
		[]byte(`{"Action":"ITEM-INFO", "Item": {"Barcode": "03010824124004"}}`))
	if err != nil {
		t.Fatal("UI failed to send message over websokcet conn")
	}

	msg = <-d.incoming
	if string(msg) != "TGC\r" {
		t.Fatal("item-info didn't trigger RFID TagCount")
	}

	d.outgoing <- []byte("OK|2\r")

	uiMsg := <-uiChan
	want := UIMsg{Action: "ITEM-INFO",
		Item: item{
			Label:   "Heavy metal in Baghdad",
			Barcode: "03010824124004",
			NumTags: 2,
		}}
	if !reflect.DeepEqual(uiMsg, want) {
		t.Errorf("Got %+v; want %+v", uiMsg, want)
		t.Fatal("UI didn't get the correct item info ")
	}

	// 1. failed write
	err = a.c.WriteMessage(websocket.TextMessage,
		[]byte(`{"Action":"WRITE", "Item": {"Barcode": "03010824124004", "NumTags": 2}}`))
	if err != nil {
		t.Fatal("UI failed to send message over websokcet conn")
	}

	msg = <-d.incoming
	if string(msg) != "SLPLBN|02030000\r" {
		t.Fatal("WRITE command didn't initialize the reader properly")
	}

	d.outgoing <- []byte("NOK\r")

	uiMsg = <-uiChan
	want = UIMsg{Action: "WRITE",
		Item: item{
			Label:       "Heavy metal in Baghdad",
			Barcode:     "03010824124004",
			WriteFailed: true,
			NumTags:     2,
		}}
	if !reflect.DeepEqual(uiMsg, want) {
		t.Errorf("Got %+v; want %+v", uiMsg, want)
		t.Fatal("UI didn't get the correct message of failed write ")
	}

	// 2. succesfull write

	err = a.c.WriteMessage(websocket.TextMessage,
		[]byte(`{"Action":"WRITE", "Item": {"Barcode": "03010824124004", "NumTags": 2}}`))
	if err != nil {
		t.Fatal("UI failed to send message over websokcet conn")
	}

	msg = <-d.incoming
	if string(msg) != "SLPLBN|02030000\r" {
		t.Fatal("WRITE command didn't initialize the reader properly")
	}

	d.outgoing <- []byte("OK\r")

	msg = <-d.incoming
	if string(msg) != "SLPLBC|NO\r" {
		t.Fatal("WRITE command didn't initialize the reader properly")
	}

	d.outgoing <- []byte("OK\r")

	msg = <-d.incoming
	if string(msg) != "SLPDTM|DS24\r" {
		t.Fatal("WRITE command didn't initialize the reader properly")
	}

	d.outgoing <- []byte("OK\r")

	msg = <-d.incoming
	if string(msg) != "SLPSSB|0\r" {
		t.Fatal("WRITE command didn't initialize the reader properly")
	}

	d.outgoing <- []byte("OK\r")

	msg = <-d.incoming
	if string(msg) != "SLPCRD|1\r" {
		t.Fatal("WRITE command didn't initialize the reader properly")
	}

	d.outgoing <- []byte("OK\r")

	msg = <-d.incoming
	if string(msg) != "SLPWTM|5000\r" {
		t.Fatal("WRITE command didn't initialize the reader properly")
	}

	d.outgoing <- []byte("OK\r")

	msg = <-d.incoming
	if string(msg) != "SLPRSS|1\r" {
		t.Fatal("WRITE command didn't initialize the reader properly")
	}

	d.outgoing <- []byte("OK\r")

	msg = <-d.incoming
	if string(msg) != "TGC\r" {
		t.Fatal("WRITE command didn't initialize the reader properly")
	}

	d.outgoing <- []byte("OK|2\r")

	msg = <-d.incoming
	if string(msg) != "WRT03010824124004|2|0\r" {
		t.Fatal("Reader didn't get the right Write command")
	}

	d.outgoing <- []byte("OK|E004010046A847AD|E004010046A847AD\r")

	uiMsg = <-uiChan
	want = UIMsg{Action: "WRITE",
		Item: item{
			Label:   "Heavy metal in Baghdad",
			Barcode: "03010824124004",
			NumTags: 2,
			Status:  "OK, preget",
		}}
	if !reflect.DeepEqual(uiMsg, want) {
		t.Errorf("Got %+v; want %+v", uiMsg, want)
		t.Fatal("UI didn't get the correct item info after WRITE command ")
	}

}

func TestUserErrors(t *testing.T) {

	// setup ->

	uiChan := make(chan UIMsg)
	sipSrv := newSIPTestServer().Failing()
	defer sipSrv.Close()

	srv := httptest.NewServer(nil)
	defer srv.Close()

	d := newDummyRFIDReader()
	defer d.Close()

	time.Sleep(50) // make sure rfidreader has got designated a port and is listening

	hub = newHub(config{
		HTTPPort:          port(srv.URL),
		SIPServer:         sipSrv.Addr(),
		TCPPort:           port(d.addr()),
		NumSIPConnections: 1,
	})
	go hub.run()
	defer hub.Close()

	a := newDummyUIAgent(uiChan, port(srv.URL))
	defer a.c.Close()

	// <- end setup

	d.outgoing <- []byte("OK\r")

	<-uiChan

	err := a.c.WriteMessage(websocket.TextMessage,
		[]byte(`{"Action":"BLA", "this is not well formed json }`))
	if err != nil {
		t.Fatal("UI failed to send message over websokcet conn")
	}

	uiMsg := <-uiChan
	want := UIMsg{Action: "CONNECT", UserError: true,
		ErrorMessage: "Failed to parse the JSON request: unexpected end of JSON input"}
	if !reflect.DeepEqual(uiMsg, want) {
		t.Errorf("Got %+v; want %+v", uiMsg, want)
	}

	// Attemp CHECKOUT without sending the patron barcode
	err = a.c.WriteMessage(websocket.TextMessage,
		[]byte(`{"Action":"CHECKOUT"}`))
	if err != nil {
		t.Fatal("UI failed to send message over websokcet conn")
	}

	uiMsg = <-uiChan
	want = UIMsg{Action: "CHECKOUT", UserError: true,
		ErrorMessage: "Patron not supplied"}
	if !reflect.DeepEqual(uiMsg, want) {
		t.Errorf("Got %+v; want %+v", uiMsg, want)
	}

}

/*
// Verify that if a second websocket connection is opened from the same IP,
// the first connection is closed.
func TestDuplicateClientConnections(t *testing.T) {
	sipPool, _ = pool.NewChannelPool(1, 1, FailingSIPResponse())

	ws, _, err := websocket.DefaultDialer.Dial("ws://127.0.0.1:8888/ws", nil)
	if err != nil {
		t.Fatal("Cannot get ws connection to 127.0.0.1:8888/ws")
	}

	_, _, err = websocket.DefaultDialer.Dial("ws://127.0.0.1:8888/ws", nil)
	if err != nil {
		t.Fatal("Cannot get ws connection to 127.0.0.1:8888/ws")
	}

	time.Sleep(100 * time.Millisecond)

	// Try writing to the first connection; it should be closed:
	err = ws.WriteMessage(websocket.TextMessage, []byte(`{"Action":"CONNECT"}`))
	if err == nil {
		t.Error("Two ws connections from the same IP should not be allowed")
	}
	time.Sleep(100 * time.Millisecond)

}
*/
