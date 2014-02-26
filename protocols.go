package main

import (
	"encoding/json"
)

// RFID-unit message protocol /////////////////////////////////////////////////

// Vendor interface which any RFID-vendor must satisfy. In order for a vendor
// to be supported, its read/write logic must be similar to what the RFIDUnit
// state-machine expects, and its protocol must be a text-based message exchange
// format. Typically, each message is terminated by \n or \r.
type Vendor interface {
	// Reset any internal state, eg. for a new read/write session
	Reset()

	// GenerateRFIDReq returns the byte-slice request to be sent to the RFID-unit.
	GenerateRFIDReq(RFIDReq) []byte

	// ParseRFIDResp parses a response from the RFID-unit.
	ParseRFIDResp([]byte) (RFIDResp, error)
}

// RFIDCommand represents the type of request to send to the RFID-unit.
type RFIDCommand uint8

const (
	cmdBeginScan  RFIDCommand = iota // BEG
	cmdEndScan                       // END
	cmdRereadTag                     // OKR
	cmdAlarmOn                       // OK1
	cmdAlarmOff                      // OK0
	cmdAlarmLeave                    // OK (leave alarm in its current state)
	cmdTagCount                      // TGC
	cmdWrite                         // WRT
)

// RFIDReq represents request to be sent to the RFID-unit.
type RFIDReq struct {
	Cmd       RFIDCommand
	WriteData []byte
}

// RFIDResp represents a parsed response from the RFID-unit.
type RFIDResp struct {
	OK         bool     // OK or NOK
	TagCount   int      // RDT<tagid>|<tagCount>
	Tag        string   // ex: RDT1003010856677001:NO:02030000 TODO strip extended ID
	WrittenIDs []string // OK|<id1>|<id2>|..
}

// UI message protocol ////////////////////////////////////////////////////////

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

func ErrorResponse(ip string, errMsg error) MsgToUI {
	return MsgToUI{
		IP:           ip,
		Action:       "ERROR",
		Message:      "Noe gikk galt, det er ikke din feil!",
		ErrorDetails: errMsg.Error(),
	}
}
