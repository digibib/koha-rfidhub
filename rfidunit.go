package main

import (
	"bufio"
	"fmt"
	"log"
	"net"
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
	UNITWaitForRetryAlarmOn
	UNITWaitForRetryAlarmOff
	UNITOff
	UNITWaitForEndOK
)

// RFIDUnit represents a connected RFID-unit.
type RFIDUnit struct {
	state          UnitState
	dept           string
	patron         string
	vendor         Vendor
	conn           net.Conn
	failedAlarmOn  map[string]string // map[Barcode]Tag
	failedAlarmOff map[string]string // map[Barcode]Tag
	currentItem    UIMsg
	items          map[string]UIMsg // Keep items around for retries
	FromUI         chan UIMsg
	ToUI           chan UIMsg
	FromRFID       chan []byte
	ToRFID         chan []byte
	Quit           chan bool
}

func newRFIDUnit(c net.Conn, send chan UIMsg) *RFIDUnit {
	return &RFIDUnit{
		state:          UNITIdle,
		vendor:         newDeichmanVendor(), // TODO get this from config
		conn:           c,
		failedAlarmOn:  make(map[string]string),
		failedAlarmOff: make(map[string]string),
		items:          make(map[string]UIMsg),
		currentItem:    UIMsg{},
		FromUI:         make(chan UIMsg),
		ToUI:           send,
		FromRFID:       make(chan []byte),
		ToRFID:         make(chan []byte),
		Quit:           make(chan bool),
	}
}

// reset checkin/checkout session
func (u *RFIDUnit) reset() {
	u.vendor.Reset()
	u.items = make(map[string]UIMsg)
	u.failedAlarmOn = make(map[string]string)
	u.failedAlarmOff = make(map[string]string)
	u.currentItem = UIMsg{}
}

// run starts the state-machine for a RFID-unit. It will shut down when the UI-
// connection is lost, on certain RFID-errors, or if it can't get a working
// connection to the SIP-server.
func (u *RFIDUnit) run() {
	var err error
	var adr = u.conn.RemoteAddr().String()
	for {
		select {
		case uiReq := <-u.FromUI:
			switch uiReq.Action {
			case "END":
				u.state = UNITWaitForEndOK
				log.Printf("[%v] UNITWaitForEndOK", adr)
				r := u.vendor.GenerateRFIDReq(RFIDReq{Cmd: cmdEndScan})
				u.ToRFID <- r
			case "ITEM-INFO":
				u.currentItem, err = DoSIPCall(sipPool, sipFormMsgItemStatus(uiReq.Item.Barcode), itemStatusParse)
				if err != nil {
					log.Println("ERROR:", err.Error())
					u.ToUI <- UIMsg{Action: "CONNECT", SIPError: true}
					u.Quit <- true
					break
				}
				u.state = UNITWaitForTagCount
				log.Printf("[%v] UNITCheckinWaitForTagCount", adr)
				u.vendor.Reset()
				r := u.vendor.GenerateRFIDReq(RFIDReq{Cmd: cmdTagCount})
				u.ToRFID <- r
			case "WRITE":
				u.state = UNITPreWriteStep1
				log.Printf("[%v] UNITPreWriteStep1", adr)
				u.currentItem.Action = "WRITE"
				u.currentItem.Item.NumTags = uiReq.Item.NumTags
				u.vendor.Reset()
				r := u.vendor.GenerateRFIDReq(RFIDReq{Cmd: cmdSLPLBN})
				u.ToRFID <- r
			case "CHECKIN":
				u.state = UNITCheckinWaitForBegOK
				u.dept = uiReq.Branch
				log.Printf("[%v] UNITCheckinWaitForBegOK", adr)
				u.reset()
				r := u.vendor.GenerateRFIDReq(RFIDReq{Cmd: cmdBeginScan})
				u.ToRFID <- r
			case "CHECKOUT":
				if uiReq.Patron == "" {
					u.ToUI <- UIMsg{Action: "CHECKOUT",
						UserError: true, ErrorMessage: "Patron not supplied"}
					u.state = UNITIdle
					log.Printf("[%v] UNITIdle", adr)
					break
				}
				u.state = UNITCheckoutWaitForBegOK
				u.patron = uiReq.Patron
				u.dept = uiReq.Branch
				log.Printf("[%v] UNITCheckoutWaitForBegOK", adr)
				u.reset()
				r := u.vendor.GenerateRFIDReq(RFIDReq{Cmd: cmdBeginScan})
				u.ToRFID <- r
			case "RETRY-ALARM-ON":
				u.state = UNITWaitForRetryAlarmOn
				log.Printf("[%v] UNITWaitForRetryAlarmOn", adr)
				for k, v := range u.failedAlarmOn {
					u.currentItem = u.items[k]
					u.currentItem.Item.Transfer = ""
					r := u.vendor.GenerateRFIDReq(RFIDReq{Cmd: cmdRetryAlarmOn, Data: []byte(v)})
					u.ToRFID <- r
					break // Remaining will be triggered in case UNITWaitForRetryAlarmOn
				}
			case "RETRY-ALARM-OFF":
				u.state = UNITWaitForRetryAlarmOff
				log.Printf("[%v] UNITWaitForRetryAlarmOff", adr)
				for k, v := range u.failedAlarmOff {
					u.currentItem = u.items[k]
					r := u.vendor.GenerateRFIDReq(RFIDReq{Cmd: cmdRetryAlarmOff, Data: []byte(v)})
					u.ToRFID <- r
					break // Remaining will be triggered in case UNITWaitForRetryAlarmOff
				}
				// TODO default case -> ERROR
			}
		case msg := <-u.FromRFID:
			r, err := u.vendor.ParseRFIDResp(msg)
			if err != nil {
				log.Println("ERROR:", err.Error())
				log.Printf("WARN: [%v] failed to understand RFID message, shutting down.", adr)
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
					log.Printf("WARN: [%v] RFID failed to start scanning, shutting down.", adr)
					u.ToUI <- UIMsg{Action: "CONNECT", RFIDError: true}
					u.Quit <- true
					break
				}
				u.state = UNITCheckin
				log.Printf("[%v] UNITCheckin", adr)
			case UNITCheckin:
				if !r.OK {
					// Missing tags case

					// Don't bother calling SIP if this is allready the current item
					if stripLeading10(r.Barcode) != u.currentItem.Item.Barcode {
						// Get item infor from SIP, to have title to display
						u.currentItem, err = DoSIPCall(sipPool, sipFormMsgItemStatus(r.Barcode), itemStatusParse)
						if err != nil {
							log.Println("ERROR:", err.Error())
							u.ToUI <- UIMsg{Action: "CONNECT", SIPError: true}
							u.Quit <- true
							break
						}
					}
					u.currentItem.Action = "CHECKIN"
					u.items[stripLeading10(r.Barcode)] = u.currentItem
					u.ToRFID <- u.vendor.GenerateRFIDReq(RFIDReq{Cmd: cmdAlarmLeave})
					u.state = UNITWaitForCheckinAlarmLeave
					log.Printf("[%v] UNITCheckinWaitForAlarmLeave", adr)
				} else {
					// Proceed with checkin transaciton
					u.currentItem, err = DoSIPCall(sipPool, sipFormMsgCheckin(u.dept, r.Barcode), checkinParse)
					if err != nil {
						log.Println("ERROR:", err.Error())
						// TODO give UI error response, and send cmdAlarmLeave to RFID
						break
					}
					if u.currentItem.Item.Unknown || u.currentItem.Item.TransactionFailed {
						u.ToRFID <- u.vendor.GenerateRFIDReq(RFIDReq{Cmd: cmdAlarmLeave})
						u.state = UNITWaitForCheckinAlarmLeave
						log.Printf("[%v] UNITWaitForCheckinAlarmLeave", adr)
					} else {
						u.items[stripLeading10(r.Barcode)] = u.currentItem
						u.failedAlarmOn[stripLeading10(r.Barcode)] = r.Tag // Store tag id for potential retry
						u.ToRFID <- u.vendor.GenerateRFIDReq(RFIDReq{Cmd: cmdAlarmOn})
						u.state = UNITWaitForCheckinAlarmOn
						log.Printf("[%v] UNITCheckinWaitForAlarmOn", adr)
					}
				}
			case UNITWaitForCheckinAlarmOn:
				u.state = UNITCheckin
				log.Printf("[%v] UNITCheckin", adr)
				if !r.OK {
					u.currentItem.Item.AlarmOnFailed = true
					u.currentItem.Item.Status = "Feil: fikk ikke skrudd på alarm."
				} else {
					delete(u.failedAlarmOn, u.currentItem.Item.Barcode)
					u.currentItem.Item.AlarmOnFailed = false
					u.currentItem.Item.Status = ""
				}
				// Discard branchcode if issuing branch is the same as target branch
				if u.dept == u.currentItem.Item.Transfer {
					u.currentItem.Item.Transfer = ""
				}
				u.ToUI <- u.currentItem
			case UNITWaitForRetryAlarmOn:
				if !r.OK {
					u.currentItem.Item.AlarmOnFailed = true
					u.currentItem.Item.Status = "Feil: fikk ikke skrudd på alarm."
				} else {
					delete(u.failedAlarmOn, u.currentItem.Item.Barcode)
					u.currentItem.Item.Status = ""
					u.currentItem.Item.AlarmOnFailed = false
				}
				u.ToUI <- u.currentItem

				if len(u.failedAlarmOn) > 0 {
					for k, v := range u.failedAlarmOn {
						u.currentItem = u.items[k]
						u.currentItem.Item.Transfer = ""
						u.state = UNITWaitForRetryAlarmOn
						log.Printf("[%v] UNITWaitForCheckoutAlarmOn", adr)
						r := u.vendor.GenerateRFIDReq(RFIDReq{Cmd: cmdRetryAlarmOn, Data: []byte(v)})
						u.ToRFID <- r
						break
					}
				} else {
					u.state = UNITCheckin
					log.Printf("[%v] UNITCheckin", adr)
				}
			case UNITWaitForCheckinAlarmLeave:
				u.state = UNITCheckin
				log.Printf("[%v] UNITCheckin", adr)
				u.currentItem.Item.Date = ""
				u.ToUI <- u.currentItem
			case UNITCheckoutWaitForBegOK:
				if !r.OK {
					log.Printf("WARN: [%v] RFID failed to start scanning, shutting down.", adr)
					u.ToUI <- UIMsg{Action: "CONNECT", RFIDError: true}
					u.Quit <- true
					break
				}
				u.state = UNITCheckout
				log.Printf("[%v] UNITCheckout", adr)
			case UNITCheckout:
				if !r.OK {
					// Missing tags case
					// TODO test this case

					// Don't bother calling SIP if this is allready the current item
					if stripLeading10(r.Barcode) != u.currentItem.Item.Barcode {
						// get status of item, to have title to display on screen,
						u.currentItem, err = DoSIPCall(sipPool, sipFormMsgItemStatus(r.Barcode), itemStatusParse)
						if err != nil {
							log.Println("ERROR:", err.Error())
							u.ToUI <- UIMsg{Action: "CONNECT", SIPError: true}
							u.Quit <- true
							break
						}
					}
					u.currentItem.Action = "CHECKOUT"
					u.items[stripLeading10(r.Barcode)] = u.currentItem
					u.ToRFID <- u.vendor.GenerateRFIDReq(RFIDReq{Cmd: cmdAlarmLeave})
					u.state = UNITWaitForCheckoutAlarmLeave
					log.Printf("[%v] UNITCheckoutWaitForAlarmLeave", adr)
				} else {
					// proced with checkout transaction
					u.currentItem, err = DoSIPCall(sipPool, sipFormMsgCheckout(u.dept, u.patron, r.Barcode), checkoutParse)
					if err != nil {
						log.Println("ERROR:", err.Error())
						// TODO give UI error response?
						break
					}
					u.currentItem.Action = "CHECKOUT"
					if u.currentItem.Item.Unknown || u.currentItem.Item.TransactionFailed {
						u.ToRFID <- u.vendor.GenerateRFIDReq(RFIDReq{Cmd: cmdAlarmLeave})
						u.state = UNITWaitForCheckoutAlarmLeave
						log.Printf("[%v] UNITCheckoutNWaitForAlarmLeave", adr)
						break
					} else {
						u.items[stripLeading10(r.Barcode)] = u.currentItem
						u.failedAlarmOff[stripLeading10(r.Barcode)] = r.Tag // Store tag id for potential retry
						u.ToRFID <- u.vendor.GenerateRFIDReq(RFIDReq{Cmd: cmdAlarmOff})
						u.state = UNITWaitForCheckoutAlarmOff
						log.Printf("[%v] UNITCheckoutNWaitForAlarmOff", adr)
					}
				}
			case UNITWaitForCheckoutAlarmOff:
				u.state = UNITCheckout
				log.Printf("[%v] UNITCheckout", adr)
				if !r.OK {
					// TODO unit-test for this
					u.currentItem.Item.AlarmOffFailed = true
					u.currentItem.Item.Status = "Feil: fikk ikke skrudd av alarm."
				} else {
					delete(u.failedAlarmOff, u.currentItem.Item.Barcode)
					u.currentItem.Item.Status = ""
					u.currentItem.Item.AlarmOffFailed = false
				}
				u.ToUI <- u.currentItem
			case UNITWaitForRetryAlarmOff:
				if !r.OK {
					u.currentItem.Item.AlarmOffFailed = true
					u.currentItem.Item.Status = "Feil: fikk ikke skrudd av alarm."
				} else {
					delete(u.failedAlarmOff, u.currentItem.Item.Barcode)
					u.currentItem.Item.Status = ""
					u.currentItem.Item.AlarmOffFailed = false
				}
				u.ToUI <- u.currentItem

				if len(u.failedAlarmOff) > 0 {
					for k, v := range u.failedAlarmOff {
						u.currentItem = u.items[k]
						u.state = UNITWaitForCheckoutAlarmOff
						log.Printf("[%v] UNITWaitForCheckoutAlarmOff", adr)
						r := u.vendor.GenerateRFIDReq(RFIDReq{Cmd: cmdRetryAlarmOff, Data: []byte(v)})
						u.ToRFID <- r
						break
					}
				} else {
					u.state = UNITCheckin
					log.Printf("[%v] UNITCheckin", adr)
				}
			case UNITWaitForCheckoutAlarmLeave:
				if !r.OK {
					// I can't imagine the RFID-reader fails to leave the
					// alarm in it current state. In any case, we continue
					log.Printf("WARN: [%v] failed to leave alarm in current state", adr)
				}
				u.state = UNITCheckout
				log.Printf("[%v] UNITCheckout", adr)
				u.ToUI <- u.currentItem
			case UNITWaitForTagCount:
				u.currentItem.Item.TransactionFailed = !r.OK
				u.state = UNITIdle
				log.Printf("[%v] UNITIdle", adr)
				u.currentItem.Action = "ITEM-INFO"
				u.currentItem.Item.NumTags = r.TagCount
				u.ToUI <- u.currentItem
			case UNITPreWriteStep1:
				if !r.OK {
					u.currentItem.Item.WriteFailed = true
					u.ToUI <- u.currentItem
					u.state = UNITIdle
					log.Printf("[%v] UNITIdle", adr)
					break
				}
				u.state = UNITPreWriteStep2
				log.Printf("[%v] UNITPreWriteStep2", adr)
				r := u.vendor.GenerateRFIDReq(RFIDReq{Cmd: cmdSLPLBC})
				u.ToRFID <- r
			case UNITPreWriteStep2:
				if !r.OK {
					u.currentItem.Item.WriteFailed = true
					u.ToUI <- u.currentItem
					u.state = UNITIdle
					log.Printf("[%v] UNITIdle", adr)
					break
				}
				u.state = UNITPreWriteStep3
				log.Printf("[%v] UNITPreWriteStep3", adr)
				r := u.vendor.GenerateRFIDReq(RFIDReq{Cmd: cmdSLPDTM})
				u.ToRFID <- r
			case UNITPreWriteStep3:
				if !r.OK {
					u.currentItem.Item.WriteFailed = true
					u.ToUI <- u.currentItem
					u.state = UNITIdle
					log.Printf("[%v] UNITIdle", adr)
					break
				}
				u.state = UNITPreWriteStep4
				log.Printf("[%v] UNITPreWriteStep4", adr)
				r := u.vendor.GenerateRFIDReq(RFIDReq{Cmd: cmdSLPSSB})
				u.ToRFID <- r
			case UNITPreWriteStep4:
				if !r.OK {
					u.currentItem.Item.WriteFailed = true
					u.ToUI <- u.currentItem
					u.state = UNITIdle
					log.Printf("[%v] UNITIdle", adr)
					break
				}
				u.state = UNITPreWriteStep5
				log.Printf("[%v] UNITPreWriteStep5", adr)
				r := u.vendor.GenerateRFIDReq(RFIDReq{Cmd: cmdSLPCRD})
				u.ToRFID <- r
			case UNITPreWriteStep5:
				if !r.OK {
					u.currentItem.Item.WriteFailed = true
					u.ToUI <- u.currentItem
					u.state = UNITIdle
					log.Printf("[%v] UNITIdle", adr)
					break
				}
				u.state = UNITPreWriteStep6
				log.Printf("[%v] UNITPreWriteStep6", adr)
				r := u.vendor.GenerateRFIDReq(RFIDReq{Cmd: cmdSLPWTM})
				u.ToRFID <- r
			case UNITPreWriteStep6:
				if !r.OK {
					u.currentItem.Item.WriteFailed = true
					u.ToUI <- u.currentItem
					u.state = UNITIdle
					log.Printf("[%v] UNITIdle", adr)
					break
				}
				u.state = UNITPreWriteStep7
				log.Printf("[%v] UNITPreWriteStep7", adr)
				r := u.vendor.GenerateRFIDReq(RFIDReq{Cmd: cmdSLPRSS})
				u.ToRFID <- r
			case UNITPreWriteStep7:
				if !r.OK {
					u.currentItem.Item.WriteFailed = true
					u.ToUI <- u.currentItem
					u.state = UNITIdle
					log.Printf("[%v] UNITIdle", adr)
					break
				}
				u.state = UNITPreWriteStep8
				log.Printf("[%v] UNITPreWriteStep8 (TGC)", adr)
				r := u.vendor.GenerateRFIDReq(RFIDReq{Cmd: cmdTagCount})
				u.ToRFID <- r
			case UNITPreWriteStep8:
				if !r.OK {
					u.currentItem.Item.WriteFailed = true
					u.ToUI <- u.currentItem
					u.state = UNITIdle
					log.Printf("[%v] UNITIdle", adr)
					break
				}
				if r.TagCount != u.currentItem.Item.NumTags {
					// Mismatch between number of tags on the RFID-reader and
					// expected number assigned in the UI.
					errMsg := fmt.Sprintf("forventet %d brikke(r), men fant %d.",
						u.currentItem.Item.NumTags, r.TagCount)
					u.currentItem.Item.Status = errMsg
					u.currentItem.Item.TagCountFailed = true
					u.ToUI <- u.currentItem
					u.state = UNITIdle
					log.Printf("[%v] UNITIdle", adr)
					break
				}
				u.state = UNITWriting
				log.Printf("[%v] UNITWriting", adr)
				r := u.vendor.GenerateRFIDReq(RFIDReq{Cmd: cmdWrite,
					Data:     []byte(u.currentItem.Item.Barcode),
					TagCount: u.currentItem.Item.NumTags})
				u.ToRFID <- r
			case UNITWriting:
				if !r.OK {
					u.currentItem.Item.WriteFailed = true
					u.ToUI <- u.currentItem
					u.state = UNITIdle
					log.Printf("[%v] UNITIdle", adr)
					break
				}
				u.state = UNITIdle
				log.Printf("[%v] UNITIdle", adr)
				u.currentItem.Item.WriteFailed = false
				u.currentItem.Item.Status = "OK, preget"
				u.ToUI <- u.currentItem
				// TODO default case -> ERROR
			}

		case <-u.Quit:
			close(u.ToRFID)
			u.state = UNITOff
			log.Printf("Shutting down RFID-unit state-machine for %v", addr2IP(adr))
			//u.ToRFID <- u.vendor.GenerateRFIDReq(RFIDReq{Cmd: cmdEndScan})
			log.Printf("Closing TCP connection to %v", adr)
			u.conn.Close()
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
			if u.state != UNITOff {
				log.Printf("ERROR: [%v] cannot read from connection: %v", u.conn.RemoteAddr().String(), err)
				//u.ToUI <- UIMsg{Action: "CONNECT", RFIDError: true}
				u.Quit <- true
			}
			break
		}
		log.Printf("<- [%v] %q", u.conn.RemoteAddr().String(), msg)
		u.FromRFID <- msg
	}
}

// tcpWriter writes messages from channel ToRFID to a TCP connection.
func (u *RFIDUnit) tcpWriter() {
	for msg := range u.ToRFID {
		_, err := u.conn.Write(msg)
		if err != nil {
			if u.state != UNITOff {
				log.Printf("ERROR: [%v] cannot write to connection: %v", u.conn.RemoteAddr().String(), err)
				u.ToUI <- UIMsg{Action: "CONNECT", RFIDError: true}
				u.Quit <- true
			}
			break
		}
		log.Printf("-> [%v] %q", u.conn.RemoteAddr().String(), msg)
	}
}
