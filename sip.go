package main

import (
	"bufio"
	"errors"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/knakk/sip"

	pool "gopkg.in/fatih/pool.v2"
)

func sipFormMsgLogin(user, pass, dept string) sip.Message {
	return sip.NewMessage(sip.MsgReqLogin).AddField(
		sip.Field{Type: sip.FieldUIDAlgorithm, Value: "0"},
		sip.Field{Type: sip.FieldPWDAlgorithm, Value: "0"},
		sip.Field{Type: sip.FieldLoginUserID, Value: user},
		sip.Field{Type: sip.FieldLoginPassword, Value: pass},
		sip.Field{Type: sip.FieldLocationCode, Value: dept},
	)
}

func sipFormMsgCheckin(dept, barcode string) sip.Message {
	now := time.Now().Format(sip.DateLayout)
	return sip.NewMessage(sip.MsgReqCheckin).AddField(
		sip.Field{Type: sip.FieldNoBlock, Value: "N"},
		sip.Field{Type: sip.FieldTransactionDate, Value: now},
		sip.Field{Type: sip.FieldReturnDate, Value: now},
		sip.Field{Type: sip.FieldCurrentLocation, Value: dept},
		sip.Field{Type: sip.FieldInstitutionID, Value: dept},
		sip.Field{Type: sip.FieldItemIdentifier, Value: barcode},
		sip.Field{Type: sip.FieldTerminalPassword, Value: ""},
	)
}

func sipFormMsgCheckout(dept, username, barcode string) sip.Message {
	now := time.Now().Format(sip.DateLayout)
	return sip.NewMessage(sip.MsgReqCheckout).AddField(
		sip.Field{Type: sip.FieldRenewalPolicy, Value: "Y"},
		sip.Field{Type: sip.FieldNoBlock, Value: "N"},
		sip.Field{Type: sip.FieldTransactionDate, Value: now},
		sip.Field{Type: sip.FieldNbDueDate, Value: now},
		sip.Field{Type: sip.FieldInstitutionID, Value: dept},
		sip.Field{Type: sip.FieldPatronIdentifier, Value: username},
		sip.Field{Type: sip.FieldItemIdentifier, Value: barcode},
		sip.Field{Type: sip.FieldTerminalPassword, Value: ""},
	)
}

func sipFormMsgItemStatus(barcode string) sip.Message {
	return sip.NewMessage(sip.MsgReqItemInformation).AddField(
		sip.Field{Type: sip.FieldTransactionDate, Value: time.Now().Format(sip.DateLayout)},
		sip.Field{Type: sip.FieldItemIdentifier, Value: barcode},
		sip.Field{Type: sip.FieldTerminalPassword, Value: ""},
		sip.Field{Type: sip.FieldInstitutionID, Value: ""},
	)
}

// A parserFunc parses a SIP response. It extracts the desired information and
// returns the JSON message to be sent to the user interface.
type parserFunc func(sip.Message) UIMsg

// DoSIPCall performs a SIP request using a SIP TCP-connection from a pool. It
// takes a SIP message as a string and a parser function to transform the SIP
// response into a UIMsg.
func DoSIPCall(p pool.Pool, msg sip.Message, parser parserFunc) (UIMsg, error) {
	// 0. Get connection from pool
	conn, err := p.Get()
	if err != nil {
		return UIMsg{}, err
	}

	// 1. Send the SIP request
	if err = msg.Encode(conn); err != nil {
		sipIDs.markAsLost(conn)
		return UIMsg{}, err
	}

	log.Printf("-> %v", strings.TrimSpace(msg.String()))

	// 2. Read SIP response

	reader := bufio.NewReader(conn)
	resp, err := reader.ReadBytes('\r')
	if err != nil {
		sipIDs.markAsLost(conn)
		return UIMsg{}, err
	}
	conn.Close()
	log.Printf("<- %v", strings.TrimSpace(string(resp)))

	// 3. Parse the response
	respMsg, err := sip.Decode(resp)
	if err != nil {
		return UIMsg{}, err
	}

	res := parser(respMsg)

	return res, nil
}

func checkinParse(msg sip.Message) UIMsg {
	var (
		fail    bool
		status  string
		unknown bool
		date    string
	)

	if msg.Field(sip.FieldOK) == "1" {
		// We only want to display date if checkin was successfull.
		date = formatDate(msg.Field(sip.FieldTransactionDate))
	} else {
		fail = true
		status = msg.Field(sip.FieldScreenMessage)
	}

	switch msg.Field(sip.FieldAlertType) {
	case "01": // reserved (on same branch)
		// TODO?
	case "02": // reserved (on other branch)
		// TODO?
	case "04": // send to other branch
		// TODO?
	case "99": // other: bad barcode / withdrawn
		unknown = true
		status = "eksemplaret finnes ikke i basen"
	}

	// Transfer either to holding branch or home branch
	branch := msg.Field(sip.FieldDestinationLocation)
	if branch == "" {
		if pl := msg.Field(sip.FieldPermanentLocation); pl != msg.Field(sip.FieldInstitutionID) {
			branch = pl
		}
	}

	return UIMsg{
		Action: "CHECKIN",
		Item: item{
			Transfer:          branch,
			Unknown:           unknown,
			TransactionFailed: fail,
			Barcode:           msg.Field(sip.FieldItemIdentifier),
			Date:              date,
			Label:             msg.Field(sip.FieldTitleIdentifier),
			Status:            status,
		},
	}
}

func checkoutParse(msg sip.Message) UIMsg {
	var (
		fail    bool
		unknown bool
		date    string
	)

	if msg.Field(sip.FieldOK) == "1" {
		// We only want to display date if checkout was successfull
		date = formatDate(msg.Field(sip.FieldTransactionDate))
	} else {
		fail = true
	}

	if msg.Field(sip.FieldTitleIdentifier) == "" {
		// TODO is this necessary?
		unknown = true
	}

	return UIMsg{
		Item: item{
			Unknown:           unknown,
			TransactionFailed: fail,
			Barcode:           msg.Field(sip.FieldItemIdentifier),
			Date:              date,
			Status:            msg.Field(sip.FieldScreenMessage),
			Label:             msg.Field(sip.FieldTitleIdentifier),
		},
	}
}

func itemStatusParse(msg sip.Message) UIMsg {
	var (
		unknown bool
		status  string
	)

	if msg.Field(sip.FieldTitleIdentifier) == "" {
		unknown = true
		status = "eksemplaret finnes ikke i basen"
	}

	return UIMsg{
		Item: item{
			TransactionFailed: true,
			Barcode:           msg.Field(sip.FieldItemIdentifier),
			Status:            status,
			Unknown:           unknown,
			Label:             msg.Field(sip.FieldTitleIdentifier),
		},
	}
}

// initSIPConn is the default factory function for creating a SIP connection.
func initSIPConn() (net.Conn, error) {
	conn, err := net.Dial("tcp", cfg.SIPServer)
	if err != nil {
		return nil, err
	}

	i := 0
	sipIDs.Lock()
	for k, v := range sipIDs.m {
		if v == nil {
			i = k
			break
		}
	}
	sipIDs.Unlock()

	if i == 0 {
		return nil, errors.New("no more IDs to create SIP login messages")
	}

	msg := sipFormMsgLogin(cfg.SIPUser, cfg.SIPPass, cfg.SIPDept)

	if err = msg.Encode(conn); err != nil {
		log.Println("ERROR:", err.Error())
		return nil, err
	}
	log.Printf("-> %v", strings.TrimSpace(msg.String()))

	reader := bufio.NewReader(conn)
	in, err := reader.ReadString('\r')
	if err != nil {
		log.Println("ERROR:", err.Error())
		return nil, err
	}

	log.Printf("<- %v", strings.TrimSpace(in))

	// fail if response == 940 (success == 941)
	if in[2] == '0' {
		return nil, errors.New("SIP login failed")
	}

	sipIDs.Lock()
	sipIDs.m[i] = conn
	sipIDs.Unlock()

	return conn, nil
}

func formatDate(s string) string {
	if len(s) < 9 {
		return s
	}
	return fmt.Sprintf("%s/%s/%s", s[6:8], s[4:6], s[0:4])
}

type sipID struct {
	sync.Mutex
	m map[int]net.Conn
}

func newSipIDs(n int) *sipID {
	s := sipID{m: make(map[int]net.Conn)}
	s.Lock()
	defer s.Unlock()
	for i := 0; i < n; i++ {
		s.m[i+1] = nil
	}
	return &s
}

func (s *sipID) markAsLost(c net.Conn) {
	s.Lock()
	defer s.Unlock()
	for k, v := range s.m {
		if v == c {
			s.m[k] = nil
			break
		}
	}
}
