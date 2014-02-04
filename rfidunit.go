package main

import (
	"bufio"
	"log"
	"net"
	"strings"
)

type RFIDUnit struct {
	conn     net.Conn
	FromUI   chan []byte
	FromRFID chan []byte
	ToRFID   chan []byte
	Quit     chan bool
}

func newRFIDUnit(c net.Conn) *RFIDUnit {
	return &RFIDUnit{
		conn:     c,
		FromUI:   make(chan []byte),
		FromRFID: make(chan []byte),
		ToRFID:   make(chan []byte),
		Quit:     make(chan bool),
	}
}

func (u *RFIDUnit) run() {
	for {
		select {
		case msg := <-u.FromRFID:
			log.Println("<- RFIDUnit:", strings.TrimRight(string(msg), "\n"))
		case msg := <-u.FromUI:
			log.Println("<- UI:", msg)
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
		log.Println("-> RFIDUnit:", strings.TrimRight(string(msg), "\n"))
		err = w.Flush()
		if err != nil {
			log.Println("ERR ", err)
			break
		}
	}
}
