package main

import (
	"bufio"
	"bytes"
	"io"
	// "io/ioutil"
	// "log"
	"net"
	"testing"
	"time"

	"github.com/knakk/specs"
)

// func init() {
// 	log.SetOutput(ioutil.Discard)
// }

// fakeTCPConn is a mock of the net.Conn interface
type fakeTCPConn struct {
	buffer bytes.Buffer
	io.ReadWriter
}

func (c fakeTCPConn) Close() error                       { return nil }
func (c fakeTCPConn) LocalAddr() net.Addr                { return nil }
func (c fakeTCPConn) RemoteAddr() net.Addr               { return nil }
func (c fakeTCPConn) SetDeadline(t time.Time) error      { return nil }
func (c fakeTCPConn) SetReadDeadline(t time.Time) error  { return nil }
func (c fakeTCPConn) SetWriteDeadline(t time.Time) error { return nil }

func fakeSIPResponse(s string) func(i interface{}) (net.Conn, error) {
	return func(i interface{}) (net.Conn, error) {
		var (
			c fakeTCPConn
			b bytes.Buffer
		)
		bufferWriter := bufio.NewWriter(&b)
		c.ReadWriter = bufio.NewReadWriter(
			bufio.NewReader(bytes.NewBufferString(s)),
			bufferWriter)
		return c, nil
	}
}

func TestFieldPairs(t *testing.T) {
	s := specs.New(t)

	fields := pairFieldIDandValue("AOHUTL|AA2|AEFillip Wahl|BLY|CQY|CC5|PCPT|PIY|AFGreetings from Koha. |\r")
	tests := []specs.Spec{
		{9, len(fields)},
		{"HUTL", fields["AO"]},
		{"2", fields["AA"]},
		{"Fillip Wahl", fields["AE"]},
		{"Y", fields["BL"]},
		{"Y", fields["CQ"]},
		{"5", fields["CC"]},
		{"PT", fields["PC"]},
		{"Y", fields["PI"]},
		{"Greetings from Koha. ", fields["AF"]},
	}
	s.ExpectAll(tests)
}

func TestSIPPatronAuthentication(t *testing.T) {
	s := specs.New(t)
	p := &ConnPool{}
	p.Init(1, fakeSIPResponse("64              01220140123    093212000000030003000000000000AOHUTL|AApatronid1|AEFillip Wahl|BLY|CQY|CC5|PCPT|PIY|AFGreetings from Koha. |\r"))

	res, err := DoSIPCall(p, sipFormMsgAuthenticate("HUTL", "patronid1", "pass"), authParse)

	s.ExpectNil(err)
	s.Expect(true, res.Authenticated)
	s.Expect("patronid1", res.Patron)
}

func TestSIPCheckin(t *testing.T) {
	s := specs.New(t)
	p := &ConnPool{}
	p.Init(1, fakeSIPResponse("101YNN20140124    093621AOHUTL|AB03011143299001|AQhvmu|AJ316 salmer og sanger|AA1|CS783.4|\r"))

	res, err := DoSIPCall(p, sipFormMsgCheckin("HUTL", "03011143299001"), checkinParse)

	s.ExpectNil(err)
	s.Expect(true, res.Item.OK)
	s.Expect("316 salmer og sanger", res.Item.Title)
	s.Expect("registrert innlevert 24/01/2014", res.Item.Status)

	p.Init(1, fakeSIPResponse("100NUY20140128    114702AO|AB234567890|CV99|AFItem not checked out|\r"))
	res, err = DoSIPCall(p, sipFormMsgCheckin("HUTL", "234567890"), checkinParse)
	s.Expect(false, res.Item.OK)
	s.Expect("Item not checked out", res.Item.Status)
}

func TestSIPCheckout(t *testing.T) {
	s := specs.New(t)
	p := &ConnPool{}
	p.Init(1, fakeSIPResponse("121NNY20140124    110740AOHUTL|AA2|AB03011174511003|AJKrutt-Kim|AH20140221    235900|\r"))
	res, err := DoSIPCall(p, sipFormMsgCheckout("2", "03011174511003"), checkoutParse)

	s.ExpectNil(err)
	s.Expect(true, res.Item.OK)
	s.Expect("Krutt-Kim", res.Item.Title)
	s.Expect("utlånt til 21/02/2014", res.Item.Status)

	p.Init(1, fakeSIPResponse("120NUN20140124    131049AOHUTL|AA2|AB1234|AJ|AH|AFInvalid Item|BLY|\r"))
	res, err = DoSIPCall(p, sipFormMsgCheckout("2", "1234"), checkoutParse)

	s.ExpectNil(err)
	s.Expect(false, res.Item.OK)
	s.Expect("Invalid Item", res.Item.Status)
}
