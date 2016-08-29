package main

type config struct {
	// Port which RFID-unit is listening on
	// TODO rename
	TCPPort string

	// Listening Port of the HTTP and WebSocket server
	HTTPPort string

	// Adress (host:port) of SIP-server
	SIPServer string

	// Credentials for SIP user to use in rfid-hub
	SIPUser string
	SIPPass string
	SIPDept string

	// Number of SIP-connections to keep in the pool
	NumSIPConnections int
}
