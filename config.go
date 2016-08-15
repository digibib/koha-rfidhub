package main

import (
	"encoding/json"
	"io/ioutil"
)

type config struct {
	// Port which RFID-unit is listening on
	TCPPort string

	// Listening Port of the HTTP and WebSocket server
	HTTPPort string

	// Log errors & warnings to this file
	ErrorLogFile string

	// Adress (host:port) of SIP-server
	SIPServer string

	// Credentials for SIP user to use in rfid-hub
	SIPUser string
	SIPPass string
	SIPDept string

	// Number of SIP-connections to keep in the pool
	NumSIPConnections int
}

func (c *config) fromFile(file string) error {
	b, err := ioutil.ReadFile(file)
	if err != nil {
		return err
	}
	err = json.Unmarshal(b, c)
	if err != nil {
		return err
	}

	return nil
}
