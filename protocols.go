package main

import "encoding/json"

type encaspulatedUIMessage struct {
	ID  string
	Msg json.RawMessage
}

type UIMessage struct {
	Type    string // "INFO" "CONNECT" or "DISCONNECT"
	Message *json.RawMessage
	ID      string // Ip:port of RFID-unit
}

type RFIDMessage struct {
	Cmd    string
	Data   string
	Status string
	ID     string
}
