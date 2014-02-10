package main

import "encoding/json"

type encaspulatedUIMessage struct {
	ID           string
	PassUnparsed bool // if true; just send directly to rfidunit, otherwise parse json and dispatch to sip
	Msg          json.RawMessage
}

// UIMessage represents a message from RFIDUnit to the test-webserver
type UIMessage struct {
	Type    string // "INFO" "CONNECT" or "DISCONNECT"
	Message *json.RawMessage
	ID      string // Ip of RFID-unit
}

// response from the state machine to UI
type UIResponse struct {
	Action        string
	Status        string
	Patron        string
	Authenticated bool
	Message       string
	ErrorDetails  string
	Item          item
	// Loans         []item
	// Holdings      []item
}

type item struct {
	Title  string // [bok] Forfatter - tittel
	Status string // forfaller 10/03/2013
	OK     bool   // false = mangler brikke / klarte ikke lese den
}
