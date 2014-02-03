package main

type UIMessage struct {
	Type    string // "INFO" "CONNECT" or "DISCONNECT"
	Message string
	ID      string // Ip:port of RFID-unit
}

type RFIDMessage struct {
	Cmd    string
	Data   string
	Status string
	ID     string
}
