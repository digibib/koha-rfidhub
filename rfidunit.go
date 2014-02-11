package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"net"
	"strings"

	"github.com/loggo/loggo"
)

/*
type UnitState uint8

const (
	UNITIdle UnitState = iota
	UNITCheckin
	UNITCheckout
	UNITWriting
)
*/

var rfidLogger = loggo.GetLogger("rfidunit")

// RFIDUnit represents a connected RFID-unit (skrankeløsning)
type RFIDUnit struct {
	conn     net.Conn
	FromUI   chan MsgFromUI
	FromRFID chan []byte
	ToRFID   chan []byte
	Quit     chan bool
	// Broadcast all events to this channel
	broadcast chan MsgToUI
}

func newRFIDUnit(c net.Conn) *RFIDUnit {
	return &RFIDUnit{
		conn:     c,
		FromUI:   make(chan MsgFromUI),
		FromRFID: make(chan []byte),
		ToRFID:   make(chan []byte),
		Quit:     make(chan bool),
	}
}

func (u *RFIDUnit) run() {
	var bMsg bytes.Buffer
	for {
		select {
		case uiReq := <-u.FromUI:
			switch uiReq.Action {
			case "RAW":
				// Pass message unparsed to RFID unit (from test webpage)
				// TODO remove when done testing
				bMsg.Write(*uiReq.RawMsg)
				bMsg.Write([]byte("\n"))
				u.ToRFID <- bMsg.Bytes()
				tcpLogger.Infof("<- UI raw msg to %v %v", uiReq.IP, string(*uiReq.RawMsg))
				bMsg.Reset()
			case "LOGIN":
				authRes, err := DoSIPCall(sipPool, sipFormMsgAuthenticate("HUTL", uiReq.Username, uiReq.PIN), authParse)
				if err != nil {
					srv.broadcast <- ErrorResponse(uiReq.IP, err)
					break
				}
				authRes.IP = uiReq.IP
				u.broadcast <- *authRes

				// bRes, err := json.Marshal(authRes)
				// if err != nil {
				// 	srv.broadcast <- ErrorResponse(uiReq.IP, err)
				// 	break
				// }
				// a.Authenticated = authRes.Authenticated
				// if a.Authenticated {
				// 	a.Patron = uiMsg.Username
				// }
				// a.ToUI <- bRes
			}
		case msg := <-u.FromRFID:
			rfidLogger.Infof("<- RFIDUnit: %v", strings.TrimSuffix(string(msg), "\n"))
			var raw = json.RawMessage(msg)
			u.broadcast <- MsgToUI{
				IP:     addr2IP(u.conn.RemoteAddr().String()),
				RawMsg: &raw,
				Action: "INFO",
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
