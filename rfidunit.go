package main

import (
	"bufio"
	"log"
	"net"
	"strings"
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

// RFIDUnit represents a connected RFID-unit (skrankeløsning)
type RFIDUnit struct {
	conn     net.Conn
	FromRFID chan []byte
	ToRFID   chan []byte
	Quit     chan bool
	// Broadcast all events to this channel
	broadcast chan encaspulatedUIMessage
}

func newRFIDUnit(c net.Conn) *RFIDUnit {
	return &RFIDUnit{
		conn:      c,
		FromRFID:  make(chan []byte),
		ToRFID:    make(chan []byte),
		Quit:      make(chan bool),
		broadcast: make(chan encaspulatedUIMessage),
	}
}

func (u *RFIDUnit) run() {
	for {
		select {
		case msg := <-u.FromRFID:
			log.Println("<- RFIDUnit:", strings.TrimSuffix(string(msg), "\n"))
			u.broadcast <- encaspulatedUIMessage{
				ID:  u.conn.RemoteAddr().String(),
				Msg: msg,
			}
		case <-u.Quit:
			// cleanup
			log.Println("INFO", "Shutting down RFID-unit statemachine:", u.conn.RemoteAddr().String())
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
			log.Println("ERR ", err)
			break
		}
		log.Println("-> RFIDUnit", u.conn.RemoteAddr().String(), strings.TrimSuffix(string(msg), "\n"))
		err = w.Flush()
		if err != nil {
			log.Println("ERR ", err)
			break
		}
	}
}
