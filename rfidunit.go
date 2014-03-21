package main

import (
	"bufio"
	"fmt"
	"net"

	"github.com/loggo/loggo"
)

// UnitState represent the current state of a RFID-unit.
type UnitState uint8

// Possible states of the RFID-unit state-machine:
const (
	UNITIdle UnitState = iota
	UNITCheckinWaitForBegOK
	UNITCheckin
	UNITCheckout
	UNITCheckoutWaitForBegOK
	UNITWaitForCheckinAlarmOn
	UNITWaitForCheckinAlarmLeave
	UNITWaitForCheckoutAlarmOff
	UNITWaitForCheckoutAlarmLeave
	UNITPreWriteStep1
	UNITPreWriteStep2
	UNITPreWriteStep3
	UNITPreWriteStep4
	UNITPreWriteStep5
	UNITPreWriteStep6
	UNITPreWriteStep7
	UNITPreWriteStep8
	UNITWriting
	UNITWaitForTagCount
	UNITOff
	UNITWaitForEndOK
)

var rfidLogger = loggo.GetLogger("rfidunit")

// RFIDUnit represents a connected RFID-unit.
type RFIDUnit struct {
	state          UnitState
	dept           string
	patron         string
	vendor         Vendor
	conn           net.Conn
	failedAlarmOn  map[string]string // map[Barcode]Tag
	failedAlarmOff map[string]string // map[Barcode]Tag
	FromUI         chan UIMsg
	ToUI           chan UIMsg
	FromRFID       chan []byte
	ToRFID         chan []byte
	Quit           chan bool
}

func newRFIDUnit(c net.Conn, send chan UIMsg) *RFIDUnit {
	ip := addr2IP(c.RemoteAddr().String())
	dept := cfg.FallBackBranch
	if branch, ok := cfg.ClientsMap[ip]; ok {
		dept = branch
	}
	return &RFIDUnit{
		state:          UNITIdle,
		dept:           dept,
		vendor:         newDeichmanVendor(), // TODO get this from config
		conn:           c,
		failedAlarmOn:  make(map[string]string),
		failedAlarmOff: make(map[string]string),
		FromUI:         make(chan UIMsg),
		ToUI:           send,
		FromRFID:       make(chan []byte),
		ToRFID:         make(chan []byte),
		Quit:           make(chan bool, 1),
	}
}

// run starts the state-machine for a RFID-unit. It will shut down when the UI-
// connection is lost, on certain RFID-errors, or if it can't get a working
// connection to the SIP-server.
func (u *RFIDUnit) run() {
	var currentItem UIMsg
	var err error
	var adr = u.conn.RemoteAddr().String()
	for {
		select {
		case uiReq := <-u.FromUI:
			switch uiReq.Action {
			case "END":
				u.state = UNITWaitForEndOK
				rfidLogger.Debugf("[%v] UNITWaitForEndOK", adr)
				r := u.vendor.GenerateRFIDReq(RFIDReq{Cmd: cmdEndScan})
				u.ToRFID <- r
			case "ITEM-INFO":
				currentItem, err = DoSIPCall(sipPool, sipFormMsgItemStatus(uiReq.Item.Barcode), itemStatusParse)
				if err != nil {
					sipLogger.Errorf(err.Error())
					u.ToUI <- UIMsg{Action: "CONNECT", SIPError: true}
					u.Quit <- true
					break
				}
				u.state = UNITWaitForTagCount
				rfidLogger.Debugf("[%v] UNITCheckinWaitForTagCount", adr)
				u.vendor.Reset()
				r := u.vendor.GenerateRFIDReq(RFIDReq{Cmd: cmdTagCount})
				u.ToRFID <- r
			case "WRITE":
				u.state = UNITPreWriteStep1
				rfidLogger.Debugf("[%v] UNITPreWriteStep1", adr)
				currentItem.Action = "WRITE"
				currentItem.Item.NumTags = uiReq.Item.NumTags
				u.vendor.Reset()
				r := u.vendor.GenerateRFIDReq(RFIDReq{Cmd: cmdSLPLBN})
				u.ToRFID <- r
			case "CHECKIN":
				u.state = UNITCheckinWaitForBegOK
				rfidLogger.Debugf("[%v] UNITCheckinWaitForBegOK", adr)
				u.vendor.Reset()
				currentItem = UIMsg{}
				r := u.vendor.GenerateRFIDReq(RFIDReq{Cmd: cmdBeginScan})
				u.ToRFID <- r
			case "CHECKOUT":
				if uiReq.Patron == "" {
					u.ToUI <- UIMsg{Action: "CHECKOUT",
						UserError: true, ErrorMessage: "Patron not supplied"}
					u.state = UNITIdle
					rfidLogger.Debugf("[%v] UNITIdle", adr)
					break
				}
				u.state = UNITCheckoutWaitForBegOK
				u.patron = uiReq.Patron
				rfidLogger.Debugf("[%v] UNITCheckoutWaitForBegOK", adr)
				u.vendor.Reset()
				currentItem = UIMsg{}
				r := u.vendor.GenerateRFIDReq(RFIDReq{Cmd: cmdBeginScan})
				u.ToRFID <- r
			case "RETRY-ALARM-ON":
				u.state = UNITWaitForCheckinAlarmOn
				rfidLogger.Debugf("[%v] UNITWaitForCheckinAlarmOn", adr)
				for _, v := range u.failedAlarmOn {
					r := u.vendor.GenerateRFIDReq(RFIDReq{Cmd: cmdRetryAlarmOn, Data: []byte(v)})
					u.ToRFID <- r
					break // Remaining will be triggered in case UNITWaitForCheckinAlarmOn
				}
			case "RETRY-ALARM-OFF":
				u.state = UNITWaitForCheckoutAlarmOff
				rfidLogger.Debugf("[%v] UNITWaitForCheckoutAlarmOff", adr)
				for _, v := range u.failedAlarmOff {
					r := u.vendor.GenerateRFIDReq(RFIDReq{Cmd: cmdRetryAlarmOff, Data: []byte(v)})
					u.ToRFID <- r
					break // Remaining will be triggered in case UNITWaitForCheckoutAlarmOff
				}
			}
		case msg := <-u.FromRFID:
			r, err := u.vendor.ParseRFIDResp(msg)
			if err != nil {
				rfidLogger.Errorf(err.Error())
				rfidLogger.Warningf("[%v] failed to understand RFID message, shutting down.", adr)
				u.ToUI <- UIMsg{Action: "CONNECT", RFIDError: true}
				u.Quit <- true
				break
			}
			switch u.state {
			case UNITWaitForEndOK:
				if !r.OK {
					// Bail out in the unlikely event of not being able to stop
					// the scan loop:
					u.ToUI <- UIMsg{Action: "CONNECT", RFIDError: true}
					u.Quit <- true
					break
				}
				u.state = UNITIdle
			case UNITCheckinWaitForBegOK:
				if !r.OK {
					rfidLogger.Warningf("[%v] RFID failed to start scanning, shutting down.", adr)
					u.ToUI <- UIMsg{Action: "CONNECT", RFIDError: true}
					u.Quit <- true
					break
				}
				u.state = UNITCheckin
				rfidLogger.Debugf("[%v] UNITCheckin", adr)
			case UNITCheckin:
				if !r.OK {
					// Missing tags case

					// Don't bother calling SIP if this is allready the current item
					if stripLeading10(r.Barcode) != currentItem.Item.Barcode {
						// Get item infor from SIP, to have title to display
						currentItem, err = DoSIPCall(sipPool, sipFormMsgItemStatus(r.Barcode), itemStatusParse)
						if err != nil {
							sipLogger.Errorf(err.Error())
							u.ToUI <- UIMsg{Action: "CONNECT", SIPError: true}
							u.Quit <- true
							break
						}
					}
					currentItem.Action = "CHECKIN"
					u.ToRFID <- u.vendor.GenerateRFIDReq(RFIDReq{Cmd: cmdAlarmLeave})
					u.state = UNITWaitForCheckinAlarmLeave
					rfidLogger.Debugf("[%v] UNITCheckinWaitForAlarmLeave", adr)
				} else {
					// Proceed with checkin transaciton
					currentItem, err = DoSIPCall(sipPool, sipFormMsgCheckin(u.dept, r.Barcode), checkinParse)
					if err != nil {
						sipLogger.Errorf(err.Error())
						// TODO give UI error response, and send cmdAlarmLeave to RFID
						break
					}
					u.failedAlarmOn[stripLeading10(r.Barcode)] = r.Tag // Store tag id for potential retry
					if currentItem.Item.Unknown || currentItem.Item.TransactionFailed {
						u.ToRFID <- u.vendor.GenerateRFIDReq(RFIDReq{Cmd: cmdAlarmLeave})
						u.state = UNITWaitForCheckinAlarmLeave
						rfidLogger.Debugf("[%v] UNITWaitForCheckinAlarmLeave", adr)

					} else {
						u.ToRFID <- u.vendor.GenerateRFIDReq(RFIDReq{Cmd: cmdAlarmOn})
						u.state = UNITWaitForCheckinAlarmOn
						rfidLogger.Debugf("[%v] UNITCheckinWaitForAlarmOn", adr)
					}
				}
			case UNITWaitForCheckinAlarmOn:
				u.state = UNITCheckin
				rfidLogger.Debugf("[%v] UNITCheckin", adr)
				if !r.OK {
					currentItem.Item.AlarmOnFailed = true
					currentItem.Item.Status = "Feil: fikk ikke skrudd på alarm."
				} else {
					delete(u.failedAlarmOn, currentItem.Item.Barcode)
					currentItem.Item.AlarmOnFailed = false
					currentItem.Item.Status = ""
					// retry others if len(u.failedAlarmOn) > 0:
					for _, v := range u.failedAlarmOn {
						u.state = UNITWaitForCheckinAlarmOn
						rfidLogger.Debugf("[%v] UNITWaitForCheckinAlarmOn", adr)
						r := u.vendor.GenerateRFIDReq(RFIDReq{Cmd: cmdRetryAlarmOn, Data: []byte(v)})
						u.ToRFID <- r
						break
					}
				}
				u.ToUI <- currentItem
			case UNITWaitForCheckinAlarmLeave:
				u.state = UNITCheckin
				rfidLogger.Debugf("[%v] UNITCheckin", adr)
				currentItem.Item.Date = ""
				u.ToUI <- currentItem
			case UNITCheckoutWaitForBegOK:
				if !r.OK {
					rfidLogger.Warningf("[%v] RFID failed to start scanning, shutting down.", adr)
					u.ToUI <- UIMsg{Action: "CONNECT", RFIDError: true}
					u.Quit <- true
					break
				}
				u.state = UNITCheckout
				rfidLogger.Debugf("[%v] UNITCheckout", adr)
			case UNITCheckout:
				if !r.OK {
					// Missing tags case
					// TODO test this case

					// Don't bother calling SIP if this is allready the current item
					if stripLeading10(r.Barcode) != currentItem.Item.Barcode {
						// get status of item, to have title to display on screen,
						currentItem, err = DoSIPCall(sipPool, sipFormMsgItemStatus(r.Barcode), itemStatusParse)
						if err != nil {
							sipLogger.Errorf(err.Error())
							u.ToUI <- UIMsg{Action: "CONNECT", SIPError: true}
							u.Quit <- true
							break
						}
					}
					currentItem.Action = "CHECKOUT"
					u.ToRFID <- u.vendor.GenerateRFIDReq(RFIDReq{Cmd: cmdAlarmLeave})
					u.state = UNITWaitForCheckoutAlarmLeave
					rfidLogger.Debugf("[%v] UNITCheckoutWaitForAlarmLeave", adr)
				} else {
					// proced with checkout transaction
					currentItem, err = DoSIPCall(sipPool, sipFormMsgCheckout(u.dept, u.patron, r.Barcode), checkoutParse)
					if err != nil {
						sipLogger.Errorf(err.Error())
						// TODO give UI error response?
						break
					}
					currentItem.Action = "CHECKOUT"
					if currentItem.Item.Unknown || currentItem.Item.TransactionFailed {
						u.ToRFID <- u.vendor.GenerateRFIDReq(RFIDReq{Cmd: cmdAlarmLeave})
						u.state = UNITWaitForCheckoutAlarmLeave
						rfidLogger.Debugf("[%v] UNITCheckoutNWaitForAlarmLeave", adr)
						break
					}
					u.failedAlarmOff[stripLeading10(r.Barcode)] = r.Tag // Store tag id for potential retry
					u.ToRFID <- u.vendor.GenerateRFIDReq(RFIDReq{Cmd: cmdAlarmOff})
					u.state = UNITWaitForCheckoutAlarmOff
					rfidLogger.Debugf("[%v] UNITCheckoutNWaitForAlarmOff", adr)
				}
			case UNITWaitForCheckoutAlarmOff:
				u.state = UNITCheckout
				rfidLogger.Debugf("[%v] UNITCheckout", adr)
				if !r.OK {
					// TODO unit-test for this
					currentItem.Item.AlarmOffFailed = true
					currentItem.Item.Status = "Feil: fikk ikke skrudd av alarm."
				} else {
					delete(u.failedAlarmOff, currentItem.Item.Barcode)
					currentItem.Item.Status = ""
					currentItem.Item.AlarmOffFailed = false
					// retry others if len(u.failedAlarmOff) > 0:
					for _, v := range u.failedAlarmOff {
						u.state = UNITWaitForCheckoutAlarmOff
						rfidLogger.Debugf("[%v] UNITWaitForCheckoutAlarmOff", adr)
						r := u.vendor.GenerateRFIDReq(RFIDReq{Cmd: cmdRetryAlarmOff, Data: []byte(v)})
						u.ToRFID <- r
						break
					}
				}
				u.ToUI <- currentItem
			case UNITWaitForCheckoutAlarmLeave:
				if !r.OK {
					// I can't imagine the RFID-reader fails to leave the
					// alarm in it current state. In any case, we continue
					rfidLogger.Warningf("[%v] failed to leave alarm in current state", adr)
				}
				u.state = UNITCheckout
				rfidLogger.Debugf("[%v] UNITCheckout", adr)
				u.ToUI <- currentItem
			case UNITWaitForTagCount:
				currentItem.Item.TransactionFailed = !r.OK
				u.state = UNITIdle
				rfidLogger.Debugf("[%v] UNITIdle", adr)
				currentItem.Action = "ITEM-INFO"
				currentItem.Item.NumTags = r.TagCount
				u.ToUI <- currentItem
			case UNITPreWriteStep1:
				if !r.OK {
					currentItem.Item.WriteFailed = true
					u.ToUI <- currentItem
					u.state = UNITIdle
					rfidLogger.Debugf("[%v] UNITIdle", adr)
					break
				}
				u.state = UNITPreWriteStep2
				rfidLogger.Debugf("[%v] UNITPreWriteStep2", adr)
				r := u.vendor.GenerateRFIDReq(RFIDReq{Cmd: cmdSLPLBC})
				u.ToRFID <- r
			case UNITPreWriteStep2:
				if !r.OK {
					currentItem.Item.WriteFailed = true
					u.ToUI <- currentItem
					u.state = UNITIdle
					rfidLogger.Debugf("[%v] UNITIdle", adr)
					break
				}
				u.state = UNITPreWriteStep3
				rfidLogger.Debugf("[%v] UNITPreWriteStep3", adr)
				r := u.vendor.GenerateRFIDReq(RFIDReq{Cmd: cmdSLPDTM})
				u.ToRFID <- r
			case UNITPreWriteStep3:
				if !r.OK {
					currentItem.Item.WriteFailed = true
					u.ToUI <- currentItem
					u.state = UNITIdle
					rfidLogger.Debugf("[%v] UNITIdle", adr)
					break
				}
				u.state = UNITPreWriteStep4
				rfidLogger.Debugf("[%v] UNITPreWriteStep4", adr)
				r := u.vendor.GenerateRFIDReq(RFIDReq{Cmd: cmdSLPSSB})
				u.ToRFID <- r
			case UNITPreWriteStep4:
				if !r.OK {
					currentItem.Item.WriteFailed = true
					u.ToUI <- currentItem
					u.state = UNITIdle
					rfidLogger.Debugf("[%v] UNITIdle", adr)
					break
				}
				u.state = UNITPreWriteStep5
				rfidLogger.Debugf("[%v] UNITPreWriteStep5", adr)
				r := u.vendor.GenerateRFIDReq(RFIDReq{Cmd: cmdSLPCRD})
				u.ToRFID <- r
			case UNITPreWriteStep5:
				if !r.OK {
					currentItem.Item.WriteFailed = true
					u.ToUI <- currentItem
					u.state = UNITIdle
					rfidLogger.Debugf("[%v] UNITIdle", adr)
					break
				}
				u.state = UNITPreWriteStep6
				rfidLogger.Debugf("[%v] UNITPreWriteStep6", adr)
				r := u.vendor.GenerateRFIDReq(RFIDReq{Cmd: cmdSLPWTM})
				u.ToRFID <- r
			case UNITPreWriteStep6:
				if !r.OK {
					currentItem.Item.WriteFailed = true
					u.ToUI <- currentItem
					u.state = UNITIdle
					rfidLogger.Debugf("[%v] UNITIdle", adr)
					break
				}
				u.state = UNITPreWriteStep7
				rfidLogger.Debugf("[%v] UNITPreWriteStep7", adr)
				r := u.vendor.GenerateRFIDReq(RFIDReq{Cmd: cmdSLPRSS})
				u.ToRFID <- r
			case UNITPreWriteStep7:
				if !r.OK {
					currentItem.Item.WriteFailed = true
					u.ToUI <- currentItem
					u.state = UNITIdle
					rfidLogger.Debugf("[%v] UNITIdle", adr)
					break
				}
				u.state = UNITPreWriteStep8
				rfidLogger.Debugf("[%v] UNITPreWriteStep8 (TGC)", adr)
				r := u.vendor.GenerateRFIDReq(RFIDReq{Cmd: cmdTagCount})
				u.ToRFID <- r
			case UNITPreWriteStep8:
				if !r.OK {
					currentItem.Item.WriteFailed = true
					u.ToUI <- currentItem
					u.state = UNITIdle
					rfidLogger.Debugf("[%v] UNITIdle", adr)
					break
				}
				if r.TagCount != currentItem.Item.NumTags {
					// Mismatch between number of tags on the RFID-reader and
					// expected number assigned in the UI.
					errMsg := fmt.Sprintf("forventet %d brikke(r), men fant %d.",
						currentItem.Item.NumTags, r.TagCount)
					currentItem.Item.Status = errMsg
					currentItem.Item.TagCountFailed = true
					u.ToUI <- currentItem
					u.state = UNITIdle
					rfidLogger.Debugf("[%v] UNITIdle", adr)
					break
				}
				u.state = UNITWriting
				rfidLogger.Debugf("[%v] UNITWriting", adr)
				r := u.vendor.GenerateRFIDReq(RFIDReq{Cmd: cmdWrite,
					Data:     []byte(currentItem.Item.Barcode),
					TagCount: currentItem.Item.NumTags})
				u.ToRFID <- r
			case UNITWriting:
				if !r.OK {
					currentItem.Item.WriteFailed = true
					u.ToUI <- currentItem
					u.state = UNITIdle
					rfidLogger.Debugf("[%v] UNITIdle", adr)
					break
				}
				u.state = UNITIdle
				rfidLogger.Debugf("[%v] UNITIdle", adr)
				currentItem.Item.WriteFailed = false
				currentItem.Item.Status = "OK, preget"
				u.ToUI <- currentItem
			}

		case <-u.Quit:
			close(u.ToRFID)
			rfidLogger.Infof("Shutting down RFID-unit state-machine for %v", addr2IP(adr))
			//u.ToRFID <- u.vendor.GenerateRFIDReq(RFIDReq{Cmd: cmdEndScan})
			rfidLogger.Infof("Closing TCP connection to %v", adr)
			u.conn.Close()
			u.state = UNITOff
			return
		}
	}
}

// tcpReader reads from a TCP connection and pipe the messages into FromRFID channel.
func (u *RFIDUnit) tcpReader() {
	r := bufio.NewReader(u.conn)
	for {
		msg, err := r.ReadBytes('\r')
		if err != nil {
			// err = io.EOF
			break
		}
		rfidLogger.Infof("<- [%v] %q", u.conn.RemoteAddr().String(), msg)
		u.FromRFID <- msg
	}
}

// tcpWriter writes messages from channel ToRFID to a TCP connection.
func (u *RFIDUnit) tcpWriter() {
	w := bufio.NewWriter(u.conn)
	for msg := range u.ToRFID {
		_, err := w.Write(msg)
		if err != nil {
			rfidLogger.Warningf(err.Error())
			break
		}
		rfidLogger.Infof("-> [%v] %q", u.conn.RemoteAddr().String(), msg)
		err = w.Flush()
		if err != nil {
			rfidLogger.Warningf(err.Error())
			break
		}
	}
}
