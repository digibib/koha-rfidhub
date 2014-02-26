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
	FromUI   chan encapsulatedUIMsg
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
		FromUI:   make(chan encapsulatedUIMsg),
		FromRFID: make(chan []byte),
		ToRFID:   make(chan []byte),
		Quit:     make(chan bool),
	}
}

func (u *RFIDUnit) run() {
	for {
		select {
		case uiReq := <-u.FromUI:
			switch uiReq.Msg.Action {
			// case "LOGIN":
			// 	authRes, err := DoSIPCall(sipPool, sipFormMsgAuthenticate(u.dept, uiReq.Username, uiReq.PIN), authParse)
			// 	if err != nil {
			// 		srv.broadcast <- ErrorResponse(uiReq.IP, err)
			// 		break
			// 	}
			// 	authRes.IP = uiReq.IP
			// 	u.broadcast <- *authRes
			// case "LOGOUT":
			// 	u.state = UNITIdle
			// 	rfidLogger.Infof("unit %v state IDLE", addr2IP(u.conn.RemoteAddr().String()))
			// 	u.ToRFID <- []byte(`{"Cmd": "SCAN-OFF"}`)
			case "CHECKIN":
				u.state = UNITCheckin
				rfidLogger.Infof("unit %v state CHECKIN", addr2IP(u.conn.RemoteAddr().String()))
				u.ToRFID <- []byte(`{"Cmd": "SCAN-ON"}`)
			case "CHECKOUT":
				u.state = UNITCheckout
				rfidLogger.Infof("unit %v state CHECKOUT", addr2IP(u.conn.RemoteAddr().String()))
				u.ToRFID <- []byte(`{"Cmd": "SCAN-ON"}` + "\n")
			}
		case msg := <-u.FromRFID:
			rfidLogger.Infof("<- RFIDUnit: %v", strings.TrimSuffix(string(msg), "\n\r"))
			_, err := u.vendor.ParseRFIDResp(msg)
			if err != nil {
				rfidLogger.Errorf(err.Error())
				break
			}
			// var raw = json.RawMessage(msg)
			// u.broadcast <- encapsulatedUIMsg{
			// 	IP:     addr2IP(u.conn.RemoteAddr().String()),
			// 	RawMsg: &raw,
			// 	Action: "INFO",
			// }
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
		msg, err := r.ReadBytes('\n')
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
		rfidLogger.Infof("-> RFIDUnit %v %v", u.conn.RemoteAddr().String(), strings.TrimSuffix(string(msg), "\n"))
		err = w.Flush()
		if err != nil {
			rfidLogger.Warningf(err.Error())
			break
		}
	}
}
