package main

import (
	"bufio"
	"fmt"
	"strings"
	"time"

	"github.com/loggo/loggo"
)

const (
	// Transaction date format
	sipDateLayout = "20060102    150405"

	// 93: Login (established SIP connection)
	sipMsg93 = "9300CNstresstest%d|COstresstest%d|CPDFB|\r"

	// 63: Patron information request
	sipMsg63 = "63012%v          AO%s|AA%s|AC<terminalpassword>|AD%s|BP000|BQ9999|\r"

	// 09: Chekin
	sipMsg09 = "09N%v%vAP%v|AO%v|AB%v|AC<terminalpassword>|\r"

	// 11: Checkout
	sipMsg11 = "11YN%v%vAO%s|AA%s|AB%s|AC<terminalpassword>|\r"

	// 17: Item status
	sipMsg17 = "17%vAO<institutionid>|AB%s|AC<terminalpassword>|\r"
)

var sipLogger = loggo.GetLogger("sip")

// TODO investigate SIP fileds, do Koha need them to be filled out?:
// <terminalpassword>
// <location>
// <institutionid>

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
func DoSIPCall(p *ConnPool, req string, parser parserFunc) (UIMsg, error) {
	// 0. Get connection from pool
	conn := p.Get()
	defer p.Release(conn)

	// 1. Send the SIP request
	_, err := conn.c.Write([]byte(req))
	if err != nil {
		p.lost <- conn // assume it is disconnected
		return UIMsg{}, err
	}

	sipLogger.Infof("-> %v", strings.TrimSpace(req))

	// 2. Read SIP response

	reader := bufio.NewReader(conn.c)
	resp, err := reader.ReadString('\r')
	if err != nil {
		return UIMsg{}, err
	}

	sipLogger.Infof("<- %v", strings.TrimSpace(resp))

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

func checkinParse(s string) UIMsg {
	a, b := s[:24], s[24:]
	var (
		ok     bool
		status string
		date   string
	)
	if a[2] == '1' {
		ok = true
	}
	fields := pairFieldIDandValue(b)
	if a[2] == '0' {
		status = fields["AF"]
	} else {
		date = fmt.Sprintf("%s/%s/%s", a[12:14], a[10:12], a[6:10])
	}
	// TODO ta med AA=patron, CS=dewey, AQ=permanent location (avdelingskode) ?
	return UIMsg{Action: "CHECKIN", Item: item{OK: ok, Barcode: fields["AB"], Date: date, Label: fields["AJ"], Status: status}}
}

func checkoutParse(s string) UIMsg {
	a, b := s[:24], s[24:]
	var (
		ok           bool
		status       string
		checkoutDate string
	)
	fields := pairFieldIDandValue(b)
	if a[2] == '1' {
		ok = true
		date := fields["AH"]
		checkoutDate = fmt.Sprintf("%s/%s/%s", date[6:8], date[4:6], date[0:4])
	} else {
		if fields["AF"] == "1" {
			status = "Failed! Don't know why. I wish the SIP-server gave us more information..."
		} else {
			status = fields["AF"]
		}
	}
	return UIMsg{Item: item{OK: ok, Barcode: fields["AB"], Date: checkoutDate, Status: status, Label: fields["AJ"]}}
}

func itemStatusParse(s string) UIMsg {
	var unknown bool
	_, b := s[:26], s[26:]
	fields := pairFieldIDandValue(b)
	if fields["AJ"] == "" {
		unknown = true
	}
	return UIMsg{Item: item{OK: false, Barcode: fields["AB"], Unknown: unknown, Label: fields["AJ"]}}
}
