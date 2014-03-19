package main

import (
	"reflect"
	"testing"
)

func TestDeichmanGenerateRFIDRequest(t *testing.T) {
	var tests = []struct {
		in  RFIDReq
		out string
	}{

		{RFIDReq{Cmd: cmdInitVersion}, "VER2.00\r"},
		{RFIDReq{Cmd: cmdBeginScan}, "BEG\r"},
		{RFIDReq{Cmd: cmdEndScan}, "END\r"},
		{RFIDReq{Cmd: cmdRereadTag}, "OKR\r"},
		{RFIDReq{Cmd: cmdAlarmOff}, "OK0\r"},
		{RFIDReq{Cmd: cmdAlarmOn}, "OK1\r"},
		{RFIDReq{Cmd: cmdAlarmLeave}, "OK \r"},
		{RFIDReq{Cmd: cmdTagCount}, "TGC\r"},
		{RFIDReq{Cmd: cmdWrite, Data: []byte("1003010650438004"), TagCount: 2}, "WRT1003010650438004|2|0\r"},
		{RFIDReq{Cmd: cmdSLPLBN}, "SLPLBN|02030000\r"},
		{RFIDReq{Cmd: cmdSLPLBC}, "SLPLBC|NO\r"},
		{RFIDReq{Cmd: cmdSLPDTM}, "SLPDTM|DS24\r"},
		{RFIDReq{Cmd: cmdSLPSSB}, "SLPSSB|0\r"},
		{RFIDReq{Cmd: cmdSLPCRD}, "SLPCRD|1\r"},
		{RFIDReq{Cmd: cmdSLPWTM}, "SLPWTM|5000\r"},
		{RFIDReq{Cmd: cmdSLPRSS}, "SLPRSS|1\r"},
		{RFIDReq{Cmd: cmdRetryAlarmOn, Data: []byte("1003010824124004:NO:02030000")}, "ACT1003010824124004:NO:02030000\r"},
	}

	v := newDeichmanVendor()

	for _, tt := range tests {
		r := string(v.GenerateRFIDReq(tt.in))
		if r != tt.out {
			t.Errorf("generateRFIDReq(%+v).Cmd => %q; want %q", tt.in, r, tt.out)
		}
	}
}

func TestDeichmanParseRFIDResp(t *testing.T) {
	var tests = []struct {
		in  string
		out RFIDResp
	}{
		{"OK\r", RFIDResp{OK: true}},
		{"NOK\r", RFIDResp{OK: false}},
		{"NOK|1\r", RFIDResp{OK: false, TagCount: 1}},
		{"NOK|2\r", RFIDResp{OK: false, TagCount: 2}},
		{"OK|2\r", RFIDResp{OK: true, TagCount: 2}},
		{"OK|12\r", RFIDResp{OK: true, TagCount: 12}},
		{"RDT1003010856677001:NO:02030000|0\r",
			RFIDResp{OK: true, Barcode: "1003010856677001", Tag: "1003010856677001:NO:02030000"}},
		{"RDT1003010856677001:NO:02030000|1\r",
			RFIDResp{OK: false, Barcode: "1003010856677001", Tag: "1003010856677001:NO:02030000"}},
	}

	v := newDeichmanVendor()

	for _, tt := range tests {
		r, err := v.ParseRFIDResp([]byte(tt.in))
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(r, tt.out) {
			t.Errorf("parseRFIDResp(%q) => %+v; want %+v", tt.in, r, tt.out)
		}
	}

	var errTests = []string{"KOK|\r", "OKI\r", "OK|Z\r"}

	for _, tt := range errTests {
		r, err := v.ParseRFIDResp([]byte(tt))
		if err == nil {
			t.Errorf("parseRFIDResp(%q) => %+v; want an error", tt, r)
		}
	}

	v.WriteMode = true

	tests = []struct {
		in  string
		out RFIDResp
	}{
		{"OK|abcd123", RFIDResp{OK: true, WrittenIDs: []string{"abcd123"}}},
		{"OK|E004010046A847AD|E004010046A847AD", RFIDResp{OK: true, WrittenIDs: []string{"E004010046A847AD", "E004010046A847AD"}}},
	}

	for _, tt := range tests {
		r, err := v.ParseRFIDResp([]byte(tt.in))
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(r, tt.out) {
			t.Errorf("parseRFIDResp(%q) => %+v; want %+v", tt.in, r, tt.out)
		}
	}

	errTests = []string{"OK|\r", "GGOK|E004004|e00414\r"}

	for _, tt := range errTests {
		r, err := v.ParseRFIDResp([]byte(tt))
		if err == nil {
			t.Errorf("parseRFIDResp(%q) => %+v; want an error", tt, r)
		}
	}

}
