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
	ID      string // Ip:port of RFID-unit
}
