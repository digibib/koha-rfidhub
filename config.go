package main

import (
	"encoding/json"
	"io/ioutil"
)

type client struct {
	IP     string // IP-address of the staff client PC
	Branch string // Branch-code to use in SIP-transactions on this client
}

type config struct {
	// Port which RFID-unit is listening on
	TCPPort string

	// Listening Port of the HTTP and WebSocket server
	HTTPPort string

	// Log levels per module
	// Example: "<root>=INFO;hub=INFO;main=INFO;sip=INFO;rfidunit=DEBUG;web=WARNING"
	LogLevels string

	// Log errors & warnings to this file
	ErrorLogFile string

	// Adress (host:port) of SIP-server
	SIPServer string

	// Number of SIP-connections to keep in the pool
	NumSIPConnections int

	// Configured clients (staff PCs with RFID-units)
	Clients []client

	// Use this branch in transactions if client IP is not in config file:
	FallBackBranch string
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
