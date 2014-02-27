package main

import (
	"bufio"
	"net"
	"strings"

	"github.com/loggo/loggo"
)

// UnitState represent the current mode of a RFID-unit.
type UnitState uint8

const (
	UNITIdle UnitState = iota
	UNITCheckin
	UNITCheckout
	UNITWriting
	UNITWaitForCheckinAlarmOn
	UNITWaitForCheckinAlarmLeave
	UNITWaitForCheckoutAlarmOff
)

var rfidLogger = loggo.GetLogger("rfidunit")

// RFIDUnit represents a connected RFID-unit (skrankeløsning)
type RFIDUnit struct {
	state    UnitState
	dept     string
	vendor   Vendor
	conn     net.Conn
	FromUI   chan UIMsg
	FromRFID chan []byte
	ToRFID   chan []byte
	Quit     chan bool
	// Broadcast all events to this channel
	broadcast chan encapsulatedUIMsg
}

func newRFIDUnit(c net.Conn) *RFIDUnit {
	return &RFIDUnit{
		state:    UNITIdle,
		dept:     "HUTL",
		vendor:   newDeichmanVendor(),
		conn:     c,
		FromUI:   make(chan UIMsg),
		FromRFID: make(chan []byte),
		ToRFID:   make(chan []byte),
		Quit:     make(chan bool),
	}
}

func (u *RFIDUnit) run() {
	var currentItem UIMsg
	var ip = u.conn.RemoteAddr().String()
	for {
		select {
		case uiReq := <-u.FromUI:
			switch uiReq.Action {
			case "CHECKIN":
				u.state = UNITCheckin
				rfidLogger.Infof("unit %v state CHECKIN", addr2IP(ip))
				u.vendor.Reset()
				r := u.vendor.GenerateRFIDReq(RFIDReq{Cmd: cmdBeginScan})
				u.ToRFID <- r
			case "CHECKOUT":
				u.state = UNITCheckout
				rfidLogger.Infof("unit %v state CHECKOUT", addr2IP(ip))
				u.vendor.Reset()
				r := u.vendor.GenerateRFIDReq(RFIDReq{Cmd: cmdBeginScan})
				u.ToRFID <- r
			}
		case msg := <-u.FromRFID:
			rfidLogger.Infof("<- RFIDUnit: %v", strings.TrimSpace(string(msg)))
			r, err := u.vendor.ParseRFIDResp(msg)
			if err != nil {
				rfidLogger.Errorf(err.Error())
				// TODO reset state? UNITIdle & u.vendor.Reset()
				break
			}
			switch u.state {
			case UNITCheckin:
				if !r.OK {
					// TODO send cmdRerad to RFIDunit??

					// get status of item, to have title to display on screen,
					currentItem, err = DoSIPCall(sipPool, sipFormMsgItemStatus(r.Tag), itemStatusParse)
					if err != nil {
						sipLogger.Errorf(err.Error())
						// TODO give UI error response?
						break
					}
					currentItem.Action = "CHECKIN"
					u.ToRFID <- u.vendor.GenerateRFIDReq(RFIDReq{Cmd: cmdAlarmLeave})
					u.state = UNITWaitForCheckinAlarmLeave
					rfidLogger.Infof("unit %v state CHECKINWaitForAlarmLeave", addr2IP(ip))
				} else {
					// proceed with checkin or checkout transaciton
					currentItem, err = DoSIPCall(sipPool, sipFormMsgCheckin("hutl", r.Tag), checkinParse)
					if err != nil {
						sipLogger.Errorf(err.Error())
						// TODO give UI error response?
						break
					}
					u.ToRFID <- u.vendor.GenerateRFIDReq(RFIDReq{Cmd: cmdAlarmOn})
					u.state = UNITWaitForCheckinAlarmOn
					rfidLogger.Infof("unit %v state CHECKINWaitForAlarmOn", addr2IP(ip))
				}
			case UNITWaitForCheckinAlarmOn:
				u.state = UNITCheckin
				rfidLogger.Infof("unit %v state CHECKIN", addr2IP(ip))
				if r.OK {
					u.broadcast <- encapsulatedUIMsg{
						IP:  addr2IP(ip),
						Msg: currentItem,
					}
				} else {
					currentItem.Item.Status = "IKKE innlevert; mangler brikke!"
				}

			case UNITWaitForCheckinAlarmLeave:
				u.state = UNITCheckin
				rfidLogger.Infof("unit %v state CHECKIN", addr2IP(ip))
				u.broadcast <- encapsulatedUIMsg{
					IP:  addr2IP(ip),
					Msg: currentItem,
				}
			case UNITCheckout:
				// TODO
			}

		case <-u.Quit:
			// cleanup
			rfidLogger.Infof("Shutting down RFID-unit run(): %v", addr2IP(ip))
			close(u.ToRFID)
			return
		}
	}
}

// read from tcp connection and pipe into FromRFID channel
func (u *RFIDUnit) tcpReader() {
	r := bufio.NewReader(u.conn)
	for {
		msg, err := r.ReadBytes('\r')
		if err != nil {
			u.Quit <- true
			break
		}
		u.FromRFID <- msg
	}
}

// write messages from channel ToRFID to tcp connection
func (u *RFIDUnit) tcpWriter() {
	w := bufio.NewWriter(u.conn)
	for msg := range u.ToRFID {
		_, err := w.Write(msg)
		if err != nil {
			rfidLogger.Warningf(err.Error())
			break
		}
		rfidLogger.Infof("-> RFIDUnit %v %v", u.conn.RemoteAddr().String(), strings.TrimSpace(string(msg)))
		err = w.Flush()
		if err != nil {
			rfidLogger.Warningf(err.Error())
			break
		}
	}
}
