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

	pool "gopkg.in/fatih/pool.v2"
)

const (
	// Transaction date format
	sipDateLayout = "20060102    150405"

	// 93: Login (established SIP connection)
	// TODO get username+password from config
	sipMsg93 = "9300CN%v|CO%v|CP%v|\r"

	// 63: Patron information request
	sipMsg63 = "63012%v          AO%s|AA%s|AC<terminalpassword>|AD%s|BP000|BQ9999|\r"

	// 09: Chekin
	sipMsg09 = "09N%v%vAP%v|AO%v|AB%v|AC<terminalpassword>|\r"

	// 11: Checkout
	sipMsg11 = "11YN%v%vAO%s|AA%s|AB%s|AC<terminalpassword>|\r"

	// 17: Item status
	sipMsg17 = "17%vAO<institutionid>|AB%s|AC<terminalpassword>|\r"
)

func sipFormMsgAuthenticate(dept, username, pin string) string {
	now := time.Now().Format(sipDateLayout)
	return fmt.Sprintf(sipMsg63, now, dept, username, pin)
}

func sipFormMsgCheckin(dept, barcode string) string {
	now := time.Now().Format(sipDateLayout)
	return fmt.Sprintf(sipMsg09, now, now, dept, dept, barcode)
}

func sipFormMsgCheckout(dept, username, barcode string) string {
	now := time.Now().Format(sipDateLayout)
	return fmt.Sprintf(sipMsg11, now, now, dept, username, barcode)
}

func sipFormMsgItemStatus(barcode string) string {
	now := time.Now().Format(sipDateLayout)
	return fmt.Sprintf(sipMsg17, now, barcode)
}

func pairFieldIDandValue(msg string) map[string]string {
	results := make(map[string]string)

	for _, pair := range strings.Split(strings.TrimRight(msg, "|\r"), "|") {
		id, val := pair[0:2], pair[2:]
		results[id] = val
	}
	return results
}

// A parserFunc parses a SIP response. It extracts the desired information and
// returns the JSON message to be sent to the user interface.
type parserFunc func(string) UIMsg

// DoSIPCall performs a SIP request using a SIP TCP-connection from a pool. It
// takes a SIP message as a string and a parser function to transform the SIP
// response into a UIMsg.
func DoSIPCall(p pool.Pool, req string, parser parserFunc) (UIMsg, error) {
	// 0. Get connection from pool
	conn, err := p.Get()
	if err != nil {
		return UIMsg{}, err
	}

	// 1. Send the SIP request
	_, err = conn.Write([]byte(req))
	if err != nil {
		sipIDs.markAsLost(conn)
		return UIMsg{}, err
	}

	log.Printf("-> %v", strings.TrimSpace(req))

	// 2. Read SIP response

	reader := bufio.NewReader(conn)
	resp, err := reader.ReadString('\r')
	if err != nil {
		sipIDs.markAsLost(conn)
		return UIMsg{}, err
	}
	conn.Close()
	log.Printf("<- %v", strings.TrimSpace(resp))

	// 3. Parse the response
	res := parser(resp)

	return res, nil
}

// func authParse(s string) UIMsg {
// 	b := s[61:] // first part of SIPresponse not needed here
// 	fields := pairFieldIDandValue(b)

// 	var auth bool
// 	if fields["CQ"] == "Y" {
// 		auth = true
// 	}
// 	return UIMsg{Action: "LOGIN", Authenticated: auth, PatronID: fields["AA"], PatronName: fields["AE"]}
// }

// TODO make parsefunctions more robust. What if len(s) < 24?
func checkinParse(s string) UIMsg {
	a, b := s[:24], s[24:]
	var (
		fail    bool
		status  string
		date    string
		unknown bool
		branch  string
	)
	fields := pairFieldIDandValue(b)
	if a[2] == '0' {
		fail = true
		status = fields["AF"]
	} else {
		date = fmt.Sprintf("%s/%s/%s", a[12:14], a[10:12], a[6:10])
	}
	if fields["CV"] == "99" {
		// The code CV99 seems to be invented by Koha's SIP server to indicate
		// invalid item.. TODO check this with someone who knows.
		unknown = true
		status = "strekkoden finnes ikke i basen"
	}
	// Transfer either to holding branch or home branch
	branch, ok := fields["CT"]
	if !ok {
		if fields["AO"] != fields["AQ"] {
			branch = fields["AQ"]
		}

	}
	// TODO ta med AA=patron, CS=dewey, AQ=permanent location (avdelingskode) ?
	return UIMsg{Action: "CHECKIN", Item: item{Transfer: branch, Unknown: unknown, TransactionFailed: fail, Barcode: fields["AB"], Date: date, Label: fields["AJ"], Status: status}}
}

func checkoutParse(s string) UIMsg {
	a, b := s[:24], s[24:]
	var (
		fail         bool
		status       string
		checkoutDate string
		unknown      bool
	)
	fields := pairFieldIDandValue(b)
	if a[2] == '1' {
		date := fields["AH"]
		checkoutDate = fmt.Sprintf("%s/%s/%s", date[6:8], date[4:6], date[0:4])
	} else {
		fail = true
		if fields["AF"] == "1" {
			status = "Failed! Don't know why. I wish the SIP-server gave us more information..."
		} else {
			status = fields["AF"]
		}
	}
	if fields["AJ"] == "" {
		unknown = true
	}
	return UIMsg{Item: item{Unknown: unknown, TransactionFailed: fail, Barcode: fields["AB"], Date: checkoutDate, Status: status, Label: fields["AJ"]}}
}

func itemStatusParse(s string) UIMsg {
	var (
		unknown bool
		status  string
	)
	_, b := s[:26], s[26:]
	fields := pairFieldIDandValue(b)
	if fields["AJ"] == "" {
		unknown = true
		status = "strekkoden finnes ikke i basen"
	}
	return UIMsg{Item: item{TransactionFailed: true, Barcode: fields["AB"], Status: status, Unknown: unknown, Label: fields["AJ"]}}
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

	out := fmt.Sprintf(sipMsg93, cfg.SIPUser, cfg.SIPPass, cfg.SIPDept)
	_, err = conn.Write([]byte(out))
	if err != nil {
		log.Println("ERROR:", err.Error())
		return nil, err
	}
	log.Printf("-> %v", strings.TrimSpace(out))

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
