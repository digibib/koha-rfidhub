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
	for {
		select {
		case uiReq := <-u.FromUI:
			switch uiReq.Action {
			case "CHECKIN":
				u.state = UNITCheckin
				rfidLogger.Infof("unit %v state CHECKIN", addr2IP(u.conn.RemoteAddr().String()))
				u.vendor.Reset()
				r := u.vendor.GenerateRFIDReq(RFIDReq{Cmd: cmdBeginScan})
				u.ToRFID <- r
			case "CHECKOUT":
				u.state = UNITCheckout
				rfidLogger.Infof("unit %v state CHECKOUT", addr2IP(u.conn.RemoteAddr().String()))
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
			if !r.OK {
				// TODO SIP get status of item, to have title to display on screen,
				// together with message (brikke mangler e.l.)
			}

			switch u.state {
			case UNITCheckin:
				sipRes, err := DoSIPCall(sipPool, sipFormMsgCheckin("hutl", "03011174511003"), checkinParse)
				if err != nil {
					sipLogger.Errorf(err.Error())
					// TODO give UI error response?
					break
				}
				u.broadcast <- encapsulatedUIMsg{
					IP:  addr2IP(u.conn.RemoteAddr().String()),
					Msg: sipRes,
				}
			}

		case <-u.Quit:
			// cleanup
			rfidLogger.Infof("Shutting down RFID-unit run(): %v", addr2IP(u.conn.RemoteAddr().String()))
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
