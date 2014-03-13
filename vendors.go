package main

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

// deichmanVendor is the RFID-vendor used on Deichman's staff PCs.
// http://it.deichman.no/projects/biblioteksystem/wiki/RFID-kommunikasjon
type deichmanVendor struct {
	TagCount  int
	WriteMode bool
}

func newDeichmanVendor() *deichmanVendor {
	return &deichmanVendor{}
}

func (v *deichmanVendor) Reset() {
	v.WriteMode = false
	v.TagCount = 0
}

func (v *deichmanVendor) GenerateRFIDReq(r RFIDReq) []byte {
	switch r.Cmd {
	case cmdInitVersion:
		return []byte("VER2.00\r")
	case cmdBeginScan:
		return []byte("BEG\r")
	case cmdEndScan:
		return []byte("END\r")
	case cmdAlarmLeave:
		return []byte("OK \r")
	case cmdAlarmOff:
		return []byte("OK0\r")
	case cmdAlarmOn:
		return []byte("OK1\r")
	case cmdRereadTag:
		return []byte("OKR\r")
	case cmdTagCount:
		return []byte("TGC\r")
	case cmdWrite:
		var b bytes.Buffer
		i := strconv.Itoa(v.TagCount)
		b.Write([]byte("WRT"))
		b.Write(r.WriteData)
		b.WriteByte('|')
		b.WriteString(i)
		b.Write([]byte("|0\r"))
		return b.Bytes()
	}

	// This can never be reached, given all cases of r.Cmd are covered above:
	return []byte("OK\r")
}

// ParseRFIDResp parses the RFID response.
func (v *deichmanVendor) ParseRFIDResp(r []byte) (RFIDResp, error) {
	s := strings.TrimSuffix(string(r), "\r")
	s = strings.TrimPrefix(s, "\n")
	l := len(s)

	switch {
	case l == 2:
		if s == "OK" {
			return RFIDResp{OK: true}, nil
		}
	case l == 3:
		if s == "NOK" {
			return RFIDResp{OK: false}, nil
		}
	case l > 3:
		if s[0:2] == "OK" {
			b := strings.Split(s, "|")
			if len(b) <= 1 {
				break
			}
			if v.WriteMode {
				// Ex: OK|E004010046A847AD|E004010046A847AD
				return RFIDResp{OK: true, WrittenIDs: b[1:len(b)]}, nil
			}
			// Ex: OK|2
			i, err := strconv.Atoi(b[1])
			if err != nil {
				break
			}
			return RFIDResp{OK: true, TagCount: i}, nil
		}
		if s[0:3] == "RDT" {
			b := strings.Split(s[3:l], "|")
			if len(b) <= 1 {
				break
			}
			var ok bool
			if b[1] == "0" {
				ok = true
			}
			if b[1] != "0" && b[1] != "1" {
				break
			}
			t := strings.Split(b[0], ":")
			return RFIDResp{OK: ok, Tag: t[0]}, nil
		}
	}

	// Fall-through case:
	return RFIDResp{}, fmt.Errorf("deichmanVendor.ParseRFIDResp: cannot parse this response: %q", r)
}
