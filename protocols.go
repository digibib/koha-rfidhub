package main

import "encoding/json"

type MsgFromUI struct {
	IP       string
	RawMsg   *json.RawMessage
	Action   string
	Username string
	PIN      string
}

type MsgToUI struct {
	IP            string
	Action        string
	Status        string
	PatronID      string
	PatronName    string
	RawMsg        *json.RawMessage
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
