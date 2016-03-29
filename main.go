package main

import (
	"net/http"
	"os"
	"strconv"

	"github.com/loggo/loggo"
	pool "gopkg.in/fatih/pool.v2"
)

// APPLICATION GLOBALS

var (
	cfg     = &config{}
	sipPool pool.Pool
	sipIDs  *sipID
	hub     *Hub
	logger  = loggo.GetLogger("main")
	status  *appMetrics
)

// APPLICATION ENTRY POINT

func main() {
	// Read config from json file
	err := cfg.fromFile("config.json")
	if err != nil {
		// or Fallback to defaults
		cfg = &config{
			TCPPort:           "6005",
			HTTPPort:          "8899",
			LogLevels:         "<root>=INFO;hub=INFO;main=INFO;sip=INFO;rfidunit=DEBUG;web=WARNING",
			ErrorLogFile:      "errors.log",
			SIPServer:         "localhost:6001",
			SIPUser:           "autouser",
			SIPPass:           "autopass",
			NumSIPConnections: 3,
			FallBackBranch:    "ukjent",
		}
		logger.Errorf("Couldn't read config file: %v", err.Error())
	}
	// Override with environment vars
	if os.Getenv("TCP_PORT") != "" {
		cfg.TCPPort = os.Getenv("TCPPORT")
	}
	if os.Getenv("HTTP_PORT") != "" {
		cfg.HTTPPort = os.Getenv("HTTPPORT")
	}
	if os.Getenv("SIP_SERVER") != "" {
		cfg.SIPServer = os.Getenv("SIP_SERVER")
	}
	if os.Getenv("SIP_USER") != "" {
		cfg.SIPUser = os.Getenv("SIP_USER")
	}
	if os.Getenv("SIP_PASS") != "" {
		cfg.SIPPass = os.Getenv("SIP_PASS")
	}
	if os.Getenv("SIP_CONNS") != "" {
		n, _ := strconv.Atoi(os.Getenv("SIP_CONNS"))
		cfg.NumSIPConnections = n
	}

	loggo.ConfigureLoggers(cfg.LogLevels)
	logger.Infof("Config: %+v", cfg)
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
	sipPool, err = pool.NewChannelPool(0, cfg.NumSIPConnections, initSIPConn)
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
