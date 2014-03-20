package main

import (
	"bufio"
	"bytes"
	"io"
	// "io/ioutil"
	// "log"
	"errors"
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

func EchoSIPResponse() func(i interface{}) (net.Conn, error) {
	return func(i interface{}) (net.Conn, error) {
		c := fakeTCPConn{}
		bufferWriter := bufio.NewWriter(&c.buffer)
		c.ReadWriter = bufio.NewReadWriter(
			bufio.NewReader(&c.buffer),
			bufferWriter)
		return c, nil
	}
}

func FailingSIPResponse() func(i interface{}) (net.Conn, error) {
	return func(i interface{}) (net.Conn, error) {
		var (
			c fakeTCPConn
			b bytes.Buffer
		)
		bufferWriter := bufio.NewWriter(&b)
		c.ReadWriter = bufio.NewReadWriter(
			bufio.NewReader(bytes.NewBufferString("")),
			bufferWriter)
		return c, nil
	}
}

func ErrorSIPResponse() func(i interface{}) (net.Conn, error) {
	return func(i interface{}) (net.Conn, error) {
		var (
			c fakeTCPConn
			b bytes.Buffer
		)
		bufferWriter := bufio.NewWriter(&b)
		c.ReadWriter = bufio.NewReadWriter(
			bufio.NewReader(bytes.NewBufferString("")),
			bufferWriter)
		return c, errors.New("cannot open SIP-connection")
	}
}

func TestFieldPairs(t *testing.T) {
	s := specs.New(t)

	fields := pairFieldIDandValue("AOHUTL|AA2|AEFillip Wahl|BLY|CQY|CC5|PCPT|PIY|ZZ|AFGreetings from Koha. |\r")
	tests := []specs.Spec{
		{10, len(fields)},
		{"HUTL", fields["AO"]},
		{"2", fields["AA"]},
		{"Fillip Wahl", fields["AE"]},
		{"Y", fields["BL"]},
		{"Y", fields["CQ"]},
		{"5", fields["CC"]},
		{"PT", fields["PC"]},
		{"Y", fields["PI"]},
		{"", fields["ZZ"]},
		{"Greetings from Koha. ", fields["AF"]},
	}
	s.ExpectAll(tests)
}

// func TestSIPPatronAuthentication(t *testing.T) {
// 	s := specs.New(t)
// 	p := &ConnPool{}
// 	p.Init(1, fakeSIPResponse("64              01220140123    093212000000030003000000000000AOHUTL|AApatronid1|AEFillip Wahl|BLY|CQY|CC5|PCPT|PIY|AFGreetings from Koha. |\r"))

// 	res, err := DoSIPCall(p, sipFormMsgAuthenticate("HUTL", "patronid1", "pass"), authParse)

// 	s.ExpectNil(err)
// 	s.Expect(true, res.Authenticated)
// 	s.Expect("patronid1", res.PatronID)
// 	s.Expect("Fillip Wahl", res.PatronName)
// }

func TestSIPCheckin(t *testing.T) {
	s := specs.New(t)
	p := &ConnPool{}
	p.initFn = fakeSIPResponse("101YNN20140124    093621AOHUTL|AB03011143299001|AQhvmu|AJ316 salmer og sanger|AA1|CS783.4|\r")
	p.Init(1)

	res, err := DoSIPCall(p, sipFormMsgCheckin("HUTL", "03011143299001"), checkinParse)

	s.ExpectNil(err)
	s.Expect(false, res.Item.TransactionFailed)
	s.Expect("316 salmer og sanger", res.Item.Label)
	s.Expect("24/01/2014", res.Item.Date)

	p.initFn = fakeSIPResponse("100NUY20140128    114702AO|AB234567890|CV99|AFItem not checked out|\r")
	p.Init(1)
	res, err = DoSIPCall(p, sipFormMsgCheckin("HUTL", "234567890"), checkinParse)
	s.Expect(true, res.Item.TransactionFailed)
	s.Expect("Item not checked out", res.Item.Status)
}

func TestSIPCheckout(t *testing.T) {
	s := specs.New(t)
	p := &ConnPool{}
	p.initFn = fakeSIPResponse("121NNY20140124    110740AOHUTL|AA2|AB03011174511003|AJKrutt-Kim|AH20140221    235900|\r")
	p.Init(1)
	res, err := DoSIPCall(p, sipFormMsgCheckout("HUTL", "2", "03011174511003"), checkoutParse)

	s.ExpectNil(err)
	s.Expect(false, res.Item.TransactionFailed)
	s.Expect("Krutt-Kim", res.Item.Label)
	s.Expect("21/02/2014", res.Item.Date)

	p.initFn = fakeSIPResponse("120NUN20140124    131049AOHUTL|AA2|AB1234|AJ|AH|AFInvalid Item|BLY|\r")
	p.Init(1)
	res, err = DoSIPCall(p, sipFormMsgCheckout("HUTL", "2", "1234"), checkoutParse)

	s.ExpectNil(err)
	s.Expect(true, res.Item.TransactionFailed)
	s.Expect("Invalid Item", res.Item.Status)
}

func TestSIPItemStatus(t *testing.T) {
	s := specs.New(t)
	p := &ConnPool{}
	p.initFn = fakeSIPResponse("1803020120140226    203140AB03010824124004|AJHeavy metal in Baghdad|AQfhol|BGfhol|\r")
	p.Init(1)
	res, err := DoSIPCall(p, sipFormMsgItemStatus("03010824124004"), itemStatusParse)

	s.ExpectNil(err)
	s.Expect(true, res.Item.TransactionFailed)
	s.Expect("Heavy metal in Baghdad", res.Item.Label)

	p.initFn = fakeSIPResponse("1801010120140228    110748AB1003010856677001|AJ|\r")
	p.Init(1)
	res, err = DoSIPCall(p, sipFormMsgItemStatus("1003010856677001"), itemStatusParse)

	s.ExpectNil(err)
	s.Expect(true, res.Item.TransactionFailed)
	s.Expect(true, res.Item.Unknown)
}
