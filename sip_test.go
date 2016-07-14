package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"gopkg.in/fatih/pool.v2"
)

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

func fakeSIPResponse(s string) func() (net.Conn, error) {
	return func() (net.Conn, error) {
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

func EchoSIPResponse() func() (net.Conn, error) {
	return func() (net.Conn, error) {
		c := fakeTCPConn{}
		bufferWriter := bufio.NewWriter(&c.buffer)
		c.ReadWriter = bufio.NewReadWriter(
			bufio.NewReader(&c.buffer),
			bufferWriter)
		return c, nil
	}
}

func FailingSIPResponse() func() (net.Conn, error) {
	return func() (net.Conn, error) {
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

func ErrorSIPResponse() func() (net.Conn, error) {
	return func() (net.Conn, error) {
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

func initFakeConn() (net.Conn, error) {
	var (
		c fakeTCPConn
		b bytes.Buffer
	)
	i := 1
	bufferWriter := bufio.NewWriter(&b)
	c.ReadWriter = bufio.NewReadWriter(
		bufio.NewReader(bytes.NewBufferString(fmt.Sprintf("result #%v\r", i))),
		bufferWriter)
	return c, nil
}

func TestFieldPairs(t *testing.T) {

	fields := pairFieldIDandValue("AOHUTL|AA2|AEFillip Wahl|BLY|CQY|CC5|PCPT|PIY|ZZ|AFGreetings from Koha. |\r")
	tests := []struct{ want, got string }{
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

	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("got %q; want %q", tt.got, tt.want)
		}
	}
}

// func TestSIPPatronAuthentication(t *testing.T) {
// 	p := &ConnPool{}
// 	p.Init(1, fakeSIPResponse("64              01220140123    093212000000030003000000000000AOHUTL|AApatronid1|AEFillip Wahl|BLY|CQY|CC5|PCPT|PIY|AFGreetings from Koha. |\r"))

// 	res, err := DoSIPCall(p, sipFormMsgAuthenticate("HUTL", "patronid1", "pass"), authParse)
//	if err != nil {
//		t.Fatal(err)
//	}
//	if res.Item.Authenticated != true {
//		t.Errorf("res.Item.Authenticated == false; want true")
//	}
//	if want := "patronid1"; res.PatronID != want {
//		t.Errorf("res.Item.PatronID == %q; want %q", res.PatronID, want)
//	}
//	if want := "Fillip Wahl"; res.PatronName != want {
//		t.Errorf("res.Item.PatronName == %q; want %q", res.PatronName, want)
//	}
// }

func TestSIPCheckin(t *testing.T) {
	p, _ := pool.NewChannelPool(1, 1, fakeSIPResponse("101YNN20140124    093621AOHUTL|AB03011143299001|AQhvmu|AJ316 salmer og sanger|AA1|CS783.4|\r"))

	res, err := DoSIPCall(p, sipFormMsgCheckin("HUTL", "03011143299001"), checkinParse)
	if err != nil {
		t.Fatal(err)
	}
	if res.Item.TransactionFailed != false {
		t.Errorf("res.Item.TransactionFailed == true; want false")
	}
	if want := "316 salmer og sanger"; res.Item.Label != want {
		t.Errorf("res.Item.Label == %q; want %q", res.Item.Label, want)
	}

	if want := "24/01/2014"; res.Item.Date != want {
		t.Errorf("res.Item.Date == %q; want %q", res.Item.Date)
	}

	p, _ = pool.NewChannelPool(1, 1, fakeSIPResponse("100NUY20140128    114702AO|AB234567890|CV99|AFItem not checked out|\r"))
	res, err = DoSIPCall(p, sipFormMsgCheckin("HUTL", "234567890"), checkinParse)
	if res.Item.TransactionFailed != true {
		t.Errorf("res.Item.TransactionFailed == false; want true")
	}
	if want := "strekkoden finnes ikke i basen"; res.Item.Status != want {
		t.Errorf("res.Item.Status == %q; want %q", res.Item.Status, want)
	}

	p, _ = pool.NewChannelPool(1, 1, fakeSIPResponse("100YNY20140511    092216AOGRY|AB03010013753001|AQhutl|AJHeksenes historie|CS272 And|CTfroa|CY11|DAåsen|CV02|AFItem not checked out|\r"))
	res, err = DoSIPCall(p, sipFormMsgCheckin("hutl", "03010013753001"), checkinParse)
	if err != nil {
		t.Fatal(err)
	}
	if want := "froa"; res.Item.Transfer != want {
		t.Errorf("res.Item.Transfer == %q; want %q", res.Item.Transfer, want)
	}
}

func TestSIPCheckout(t *testing.T) {
	p, _ := pool.NewChannelPool(1, 1, fakeSIPResponse("121NNY20140124    110740AOHUTL|AA2|AB03011174511003|AJKrutt-Kim|AH20140221    235900|\r"))
	res, err := DoSIPCall(p, sipFormMsgCheckout("HUTL", "2", "03011174511003"), checkoutParse)
	if err != nil {
		t.Fatal(err)
	}
	if res.Item.TransactionFailed != false {
		t.Errorf("res.Item.TransactionFailed == true; want false")
	}
	if want := "Krutt-Kim"; res.Item.Label != want {
		t.Errorf("res.Item.Label == %q; want %q", res.Item.Label, want)
	}
	if want := "21/02/2014"; res.Item.Date != want {
		t.Errorf("res.Item.Date == %q; want %q", res.Item.Date)
	}

	p, _ = pool.NewChannelPool(1, 1, fakeSIPResponse("120NUN20140124    131049AOHUTL|AA2|AB1234|AJ|AH|AFInvalid Item|BLY|\r"))
	res, err = DoSIPCall(p, sipFormMsgCheckout("HUTL", "2", "1234"), checkoutParse)
	if err != nil {
		t.Fatal(err)
	}
	if res.Item.TransactionFailed != true {
		t.Errorf("res.Item.TransactionFailed == false; want true")
	}
	if want := "Invalid Item"; res.Item.Status != want {
		t.Errorf("res.Item.Status == %q; want %q", res.Item.Status, want)
	}
}

func TestSIPItemStatus(t *testing.T) {
	p, _ := pool.NewChannelPool(1, 1, fakeSIPResponse("1803020120140226    203140AB03010824124004|AJHeavy metal in Baghdad|AQfhol|BGfhol|\r"))
	res, err := DoSIPCall(p, sipFormMsgItemStatus("03010824124004"), itemStatusParse)
	if err != nil {
		t.Fatal(err)
	}
	if res.Item.TransactionFailed != true {
		t.Errorf("res.Item.TransactionFailed == false; want true")
	}

	p, _ = pool.NewChannelPool(1, 1, fakeSIPResponse("1801010120140228    110748AB1003010856677001|AJ|\r"))
	res, err = DoSIPCall(p, sipFormMsgItemStatus("1003010856677001"), itemStatusParse)
	if err != nil {
		t.Fatal(err)
	}
	if res.Item.TransactionFailed != true {
		t.Errorf("res.Item.TransactionFailed == false; want true")
	}
	if res.Item.Unknown != true {
		t.Errorf("res.Item.Unknown == false; want true")
	}
}
