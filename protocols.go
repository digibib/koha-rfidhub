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

	// Initialize writer commands.
	// SLP (Set Library Paramter) commands. Reader returns OK or NOK.
	cmdSLPLBN // SLPLBN|02030000 (LBN: library number)
	cmdSLPLBC // SLPLBC|NO       (LBC: library country code)
	cmdSLPDTM // SLPDTM|DS24     (DTM: data model, "Danish Standard" / ISO28560−3)
	cmdSLPSSB // SLPSSB|0        (SSB: set security bit when writing, 0: Reset, 1: Set)
	cmdSLPCRD // SLPCRD|1        (CRD: check read after write: 0: No, 1: Yes)
	cmdSLPWTM // SLPWTM|5000     (WTM: time to wait for "setsize" tags to be available, in ms)
	cmdSLPRSS // SLPRSS|1        (RSS: return set status, status value for 1−tag−only sets,
	//                  0: complete, 1: not complete, 2: complete but check manually)

	// The following are not used yet
	cmdSLPEID // SLPEID|1        (EID: send extended ID, 0: No, 1: Yes − include library number and country code)
	cmdSLPESP // SLPESP|:        (ESP: extended ID seperator: default character ’:’)
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
	Label    string
	Barcode  string
	Date     string // Format: 10/03/2013
	Status   string // An error explanation or an error message passed on from SIP-server
	Transfer string // Branchcode, or empty string if item belongs to the issuing branch
	NumTags  int

	// Possible errors
	Unknown           bool // true if SIP server cant give any information on a given barcode
	TransactionFailed bool // true if the transaction failed
	AlarmOnFailed     bool // true if it failed to turn on alarm
	AlarmOffFailed    bool // true if it failed to turn off alarm
	WriteFailed       bool // true if write to tag failed
	TagCountFailed    bool // true if mismatch between expected number of tags and found tags
}

// UIMsg is a message to or from Koha's user interface.
type UIMsg struct {
	Action       string // CHECKIN/CHECKOUT/CONNECT/ITEM-INFO/RETRY-ALARM-ON/RETRY-ALARM-OFF/WRITE/END
	Patron       string // Patron username/barcode
	RFIDError    bool   // true if RFID-reader is unavailable
	SIPError     bool   // true if SIP-server is unavailable
	UserError    bool   // true if user is not using the API correctly
	ErrorMessage string // textual description of the error
	Item         item
}
