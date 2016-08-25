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

func TestSIPCheckin(t *testing.T) {
	p, _ := pool.NewChannelPool(1, 1, fakeSIPResponse("101YNN20140124    093621AOHUTL|AB03011143299001|AQhvmu|AJ316 salmer og sanger|AA1|CS783.4|\r"))

	res, err := DoSIPCall(p, sipFormMsgCheckin("HUTL", "03011143299001"), checkinParse)
	if err != nil {
		t.Fatal(err)
	}
	if res.Item.TransactionFailed {
		t.Errorf("res.Item.TransactionFailed == true; want false")
	}
	if want := "316 salmer og sanger"; res.Item.Label != want {
		t.Errorf("res.Item.Label == %q; want %q", res.Item.Label, want)
	}

	if want := "24/01/2014"; res.Item.Date != want {
		t.Errorf("res.Item.Date == %q; want %q", res.Item.Date, want)
	}

	p, _ = pool.NewChannelPool(1, 1, fakeSIPResponse("100NUY20140128    114702AO|AB234567890|CV99|AFItem not checked out|\r"))
	res, err = DoSIPCall(p, sipFormMsgCheckin("HUTL", "234567890"), checkinParse)
	if !res.Item.TransactionFailed {
		t.Errorf("res.Item.TransactionFailed == false; want true")
	}
	if want := "eksemplaret finnes ikke i basen"; res.Item.Status != want {
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
	if res.Item.TransactionFailed {
		t.Errorf("res.Item.TransactionFailed == true; want false")
	}
	if want := "Krutt-Kim"; res.Item.Label != want {
		t.Errorf("res.Item.Label == %q; want %q", res.Item.Label, want)
	}
	if want := "24/01/2014"; res.Item.Date != want {
		t.Errorf("res.Item.Date == %q; want %q", res.Item.Date, want)
	}

	p, _ = pool.NewChannelPool(1, 1, fakeSIPResponse("120NUN20140124    131049AOHUTL|AA2|AB1234|AJ|AH|AFInvalid Item|BLY|\r"))
	res, err = DoSIPCall(p, sipFormMsgCheckout("HUTL", "2", "1234"), checkoutParse)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Item.TransactionFailed {
		t.Errorf("res.Item.TransactionFailed == false; want true")
	}
	if want := "Invalid Item"; res.Item.Status != want {
		t.Errorf("res.Item.Status == %q; want %q", res.Item.Status, want)
	}
}

func TestSIPItemStatus(t *testing.T) {
	p, _ := pool.NewChannelPool(1, 1, fakeSIPResponse("1803020120140226    203140AB03010824124004|AO|AJHeavy metal in Baghdad|AQfhol|BGfhol|\r"))
	res, err := DoSIPCall(p, sipFormMsgItemStatus("03010824124004"), itemStatusParse)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Item.TransactionFailed {
		t.Errorf("res.Item.TransactionFailed == false; want true")
	}

	p, _ = pool.NewChannelPool(1, 1, fakeSIPResponse("1801010120140228    110748AB1003010856677001|AO|AJ|\r"))
	res, err = DoSIPCall(p, sipFormMsgItemStatus("1003010856677001"), itemStatusParse)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Item.TransactionFailed {
		t.Errorf("res.Item.TransactionFailed == false; want true")
	}
	if res.Item.Unknown != true {
		t.Errorf("res.Item.Unknown == false; want true")
	}
}
