package main

import (
	"bufio"
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
	UNITWriting
	UNITWaitForCheckinAlarmOn
	UNITWaitForCheckinAlarmLeave
	UNITWaitForCheckoutAlarmOff
	UNITWaitForCheckoutAlarmLeave
	UNITOff
)

var rfidLogger = loggo.GetLogger("rfidunit")

// RFIDUnit represents a connected RFID-unit.
type RFIDUnit struct {
	state    UnitState
	dept     string
	patron   string
	vendor   Vendor
	conn     net.Conn
	FromUI   chan UIMsg
	ToUI     chan UIMsg
	FromRFID chan []byte
	ToRFID   chan []byte
	Quit     chan bool
}

func newRFIDUnit(c net.Conn, send chan UIMsg) *RFIDUnit {
	ip := addr2IP(c.RemoteAddr().String())
	dept := cfg.FallBackBranch
	if branch, ok := cfg.ClientsMap[ip]; ok {
		dept = branch
	}
	return &RFIDUnit{
		state:    UNITIdle,
		dept:     dept,
		vendor:   newDeichmanVendor(), // TODO get this from config
		conn:     c,
		FromUI:   make(chan UIMsg),
		ToUI:     send,
		FromRFID: make(chan []byte),
		ToRFID:   make(chan []byte),
		Quit:     make(chan bool, 1),
	}
}

// run starts the state-machine for a RFID-unit. It will shut down when the UI-
// connection is lost, on certain RFID-errors, or if it can't get a working
// connection to the SIP-server.
func (u *RFIDUnit) run() {
	var currentItem UIMsg
	var adr = u.conn.RemoteAddr().String()
	for {
		select {
		case uiReq := <-u.FromUI:
			switch uiReq.Action {
			case "CHECKIN":
				u.state = UNITCheckinWaitForBegOK
				rfidLogger.Infof("[%v] UNITCheckinWaitForBegOK", adr)
				u.vendor.Reset()
				r := u.vendor.GenerateRFIDReq(RFIDReq{Cmd: cmdBeginScan})
				u.ToRFID <- r
			case "CHECKOUT":
				u.state = UNITCheckoutWaitForBegOK
				// TODO return error if Patron == ""
				u.patron = uiReq.Patron
				rfidLogger.Infof("[%v] UNITCheckoutWaitForBegOK", adr)
				u.vendor.Reset()
				r := u.vendor.GenerateRFIDReq(RFIDReq{Cmd: cmdBeginScan})
				u.ToRFID <- r
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
			case UNITCheckinWaitForBegOK:
				if !r.OK {
					rfidLogger.Warningf("[%v] RFID failed to start scanning, shutting down.", adr)
					u.ToUI <- UIMsg{Action: "CONNECT", RFIDError: true}
					u.Quit <- true
					break
				}
				u.state = UNITCheckin
				rfidLogger.Infof("[%v] UNITCheckin", adr)
			case UNITCheckin:
				if !r.OK {
					// Missing tags case

					// Don't bother calling SIP if this is allready the current item
					if stripLeading10(r.Tag) != currentItem.Item.Barcode {
						// Get item infor from SIP, to have title to display
						currentItem, err = DoSIPCall(sipPool, sipFormMsgItemStatus(r.Tag), itemStatusParse)
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
					rfidLogger.Infof("[%v] UNITCheckinWaitForAlarmLeave", adr)
				} else {
					// Proceed with checkin transaciton
					currentItem, err = DoSIPCall(sipPool, sipFormMsgCheckin(u.dept, r.Tag), checkinParse)
					if err != nil {
						sipLogger.Errorf(err.Error())
						// TODO give UI error response, and send cmdAlarmLeave to RFID
						break
					}
					u.ToRFID <- u.vendor.GenerateRFIDReq(RFIDReq{Cmd: cmdAlarmOn})
					u.state = UNITWaitForCheckinAlarmOn
					rfidLogger.Infof("[%v] UNITCheckinWaitForAlarmOn", adr)
				}
			case UNITWaitForCheckinAlarmOn:
				u.state = UNITCheckin
				rfidLogger.Infof("[%v] UNITCheckin", adr)
				if !r.OK {
					currentItem.Item.Status = "IKKE innlevert"
				}
				u.ToUI <- currentItem
			case UNITWaitForCheckinAlarmLeave:
				u.state = UNITCheckin
				rfidLogger.Infof("[%v] UNITCheckin", adr)
				currentItem.Item.Date = ""
				currentItem.Item.Status = "IKKE innlevert"
				currentItem.Item.OK = false
				u.ToUI <- currentItem
			case UNITCheckoutWaitForBegOK:
				if !r.OK {
					rfidLogger.Warningf("[%v] RFID failed to start scanning, shutting down.", adr)
					u.ToUI <- UIMsg{Action: "CONNECT", RFIDError: true}
					u.Quit <- true
					break
				}
				u.state = UNITCheckout
				rfidLogger.Infof("[%v] UNITCheckout", adr)
			case UNITCheckout:
				if !r.OK {
					// Missing tags case
					// TODO test this case

					// Don't bother calling SIP if this is allready the current item
					if stripLeading10(r.Tag) != currentItem.Item.Barcode {
						// get status of item, to have title to display on screen,
						currentItem, err = DoSIPCall(sipPool, sipFormMsgItemStatus(r.Tag), itemStatusParse)
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
					rfidLogger.Infof("[%v] UNITCheckoutWaitForAlarmLeave", adr)
				} else {
					// proced with checkout transaction
					currentItem, err = DoSIPCall(sipPool, sipFormMsgCheckout(u.dept, u.patron, r.Tag), checkoutParse)
					if err != nil {
						sipLogger.Errorf(err.Error())
						// TODO give UI error response?
						break
					}
					currentItem.Action = "CHECKOUT"
					if !currentItem.Item.OK {
						u.ToRFID <- u.vendor.GenerateRFIDReq(RFIDReq{Cmd: cmdAlarmLeave})
						u.state = UNITWaitForCheckoutAlarmLeave
						rfidLogger.Infof("[%v] UNITCheckoutNWaitForAlarmLeave", adr)
						break
					}
					u.ToRFID <- u.vendor.GenerateRFIDReq(RFIDReq{Cmd: cmdAlarmOff})
					u.state = UNITWaitForCheckoutAlarmOff
					rfidLogger.Infof("[%v] UNITCheckoutNWaitForAlarmOff", adr)
				}
			case UNITWaitForCheckoutAlarmOff:
				u.state = UNITCheckout
				rfidLogger.Infof("[%v] UNITCheckout", adr)
				if !r.OK {
					// TODO unit-test for this
					currentItem.Item.OK = false
					currentItem.Item.Status = "Får ikke skrudd av alarm!"
				}
				u.ToUI <- currentItem
			case UNITWaitForCheckoutAlarmLeave:
				if !r.OK {
					// TODO quit
					break
				}
				u.state = UNITCheckout
				rfidLogger.Infof("[%v] UNITCheckout", adr)
				//currentItem.Item.Status = "IKKE innlevert"
				u.ToUI <- currentItem
			}

		case <-u.Quit:
			// cleanup
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
