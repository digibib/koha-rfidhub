package main

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
	cmdInitVersion RFIDCommand = iota // VER2
	cmdBeginScan                      // BEG
	cmdEndScan                        // END
	cmdRereadTag                      // OKR
	cmdAlarmOn                        // OK1
	cmdAlarmOff                       // OK0
	cmdAlarmLeave                     // OK (leave alarm in its current state)
	cmdTagCount                       // TGC
	cmdWrite                          // WRT
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
// For communication between Koha's web intranet interface and the RFID-hub.

type item struct {
	Label  string // [bok] Forfatter - tittel
	Date   string // 10/03/2013
	Status string
	OK     bool // false = mangler brikke / klarte ikke lese den
}

// UIMsg is a message to or from Koha's user interface.
type UIMsg struct {
	Action    string // CHECKIN/CHECKOUT/ERROR
	RFIDError bool
	SIPError  bool
	Item      item
}
