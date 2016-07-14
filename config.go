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

	// Configured clients (staff PCs with RFID-units)
	Clients []client

	// Get branch from client IP. Key = IP, Value = Branch
	ClientsMap map[string]string

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
	c.ClientsMap = make(map[string]string)
	for _, cl := range cfg.Clients {
		c.ClientsMap[cl.IP] = cl.Branch
	}
	return nil
}
