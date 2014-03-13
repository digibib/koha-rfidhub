package main

import (
	"net/http"
	"os"

	"github.com/loggo/loggo"

	_ "net/http/pprof"
)

// APPLICATION STATE

var (
	cfg     = &config{}
	sipPool *ConnPool
	hub     *Hub
	logger  = loggo.GetLogger("main")
)

// APPLICATION ENTRY POINT

func main() {
	// SETUP
	err := cfg.fromFile("config.json")
	if err != nil {
		cfg = &config{
			TCPPort:           "6005",
			HTTPPort:          "8899",
			SIPServer:         "knakk:6001",
			NumSIPConnections: 5,
			LogLevels:         "<root>=INFO;hub=INFO;main=INFO;sip=INFO;rfidunit=DEBUG;web=WARNING",
		}
		logger.Warningf("No config.json file found, using standard values")
	}
	loggo.ConfigureLoggers(cfg.LogLevels)
	file, err := os.Create("errors.log")
	if err == nil {
		err = loggo.RegisterWriter("file",
			loggo.NewSimpleWriter(file, &loggo.DefaultFormatter{}), loggo.WARNING)
		if err != nil {
			logger.Warningf(err.Error())
		}
	}

	hub = newHub()

	// START SERVICES

	logger.Infof("Creating SIP Connection pool with size: %v", cfg.NumSIPConnections)
	sipPool = NewSIPConnPool(cfg.NumSIPConnections)
	go sipPool.Monitor()

	logger.Infof("Starting Websocket hub")
	go hub.run()

	http.HandleFunc("/ws", wsHandler)

	logger.Infof("Starting HTTP server, listening at port %v", cfg.HTTPPort)
	http.ListenAndServe(":"+cfg.HTTPPort, nil)
}
