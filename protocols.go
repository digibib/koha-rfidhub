package main

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

// RFID-unit message protocol /////////////////////////////////////////////////

// RFIDCommand represents the type of request to send to the RFID-unit.
type RFIDCommand uint8

const (
	cmdInitVersion RFIDCommand = iota
	cmdBeginScan
	cmdEndScan
	cmdRereadTag
	cmdAlarmOn
	cmdRetryAlarmOn
	cmdAlarmOff
	cmdRetryAlarmOff
	cmdAlarmLeave
	cmdTagCount
	cmdWrite

	// Initialize writer commands. Reader returns OK or NOK.
	// TODO choose better names when/if we get documentation
	cmdSLPLBN // SLPLBN|02030000 (library number)
	cmdSLPLBC // SLPLBC|NO (country code)
	cmdSLPDTM // SLPDTM|DS24 (the Danish standard)
	cmdSLPSSB // SLPSSB|0 (?)
	cmdSLPCRD // SLPCRD|1 (?)
	cmdSLPWTM // SLPWTM|5000 (timeout in ms)
	cmdSLPRSS // SLPRSS|1 (?)
)

// RFIDReq represents request to be sent to the RFID-unit.
type RFIDReq struct {
	Cmd      RFIDCommand
	Data     []byte
	TagCount int
}

// RFIDResp represents a parsed response from the RFID-unit.
type RFIDResp struct {
	OK         bool
	TagCount   int
	Tag        string // 1003010530352001:NO:02030000
	Barcode    string // 1003010530352001
	WrittenIDs []string
}

// UI message protocol ////////////////////////////////////////////////////////

// For communication between Koha's web intranet interface and the RFID-hub.

type item struct {
	Label   string
	Barcode string
	Date    string // Format: 10/03/2013
	Status  string
	NumTags int
	Unknown bool // true if SIP server cant give any information
	OK      bool // true if the transaction succeded
}

// UIMsg is a message to or from Koha's user interface.
type UIMsg struct {
	Action    string // CHECKIN/CHECKOUT/CONNECT/WRITE
	Patron    string
	RFIDError bool
	SIPError  bool
	Item      item
}
