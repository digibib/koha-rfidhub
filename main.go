package main

import (
	"net/http"
	"os"

	"github.com/fatih/pool"
	"github.com/loggo/loggo"
)

// APPLICATION GLOBALS

var (
	cfg     = &config{}
	sipPool *pool.Pool
	sipIDs  *sipID
	hub     *Hub
	logger  = loggo.GetLogger("main")
	status  *appMetrics
)

// APPLICATION ENTRY POINT

func main() {
	// SETUP
	err := cfg.fromFile("config.json")
	if err != nil {
		cfg = &config{
			TCPPort:           "6005",
			HTTPPort:          "8899",
			LogLevels:         "<root>=INFO;hub=INFO;main=INFO;sip=INFO;rfidunit=DEBUG;web=WARNING",
			ErrorLogFile:      "errors.log",
			SIPServer:         "localhost:6001",
			NumSIPConnections: 3,
			FallBackBranch:    "ukjent",
		}
		logger.Infof("No config.json file found, using standard values:\n")
		logger.Infof("%#v", cfg)
	}
	loggo.ConfigureLoggers(cfg.LogLevels)
	logger.Infof("%#v", cfg)
	file, err := os.Create(cfg.ErrorLogFile)
	if err == nil {
		err = loggo.RegisterWriter("file",
			loggo.NewSimpleWriter(file, &loggo.DefaultFormatter{}), loggo.WARNING)
		if err != nil {
			logger.Warningf(err.Error())
		}
	}

	hub = newHub()
	status = registerMetrics()

	// START SERVICES
	sipIDs = newSipIDs(cfg.NumSIPConnections)
	logger.Infof("Creating SIP Connection pool with size: %v", cfg.NumSIPConnections)
	sipPool, err = pool.New(1, cfg.NumSIPConnections, initSIPConn)
	if err != nil {
		logger.Errorf(err.Error())
		os.Exit(1)
	}

	logger.Infof("Starting Websocket hub")
	go hub.run()

	http.HandleFunc("/.status", statusHandler)
	http.HandleFunc("/ws", wsHandler)

	logger.Infof("Starting HTTP server, listening at port %v", cfg.HTTPPort)
	http.ListenAndServe(":"+cfg.HTTPPort, nil)
}
