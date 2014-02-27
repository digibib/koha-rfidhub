package main

import (
	"html/template"
	"net/http"
	"os"

	"github.com/loggo/loggo"

	_ "net/http/pprof"
)

// APPLICATION STATE

var (
	cfg        = &config{}
	srv        *TCPServer
	sipPool    *ConnPool
	hub        *Hub
	templates  = template.Must(template.ParseFiles("index.html", "uitest.html"))
	logger     = loggo.GetLogger("main")
	rootLogger = loggo.GetLogger("") // TODO what is this, remove?
)

// APPLICATION ENTRY POINT

func main() {
	// SETUP
	err := cfg.fromFile("config.json")
	if err != nil {
		cfg = &config{
			TCPPort:           "6005",
			HTTPPort:          "8899",
			SIPServer:         "wombat:6001",
			NumSIPConnections: 5,
			LogLevels:         "<root>=WARNING;tcp=INFO;ws=INFO;main=INFO;sip=INFO;rfidunit=DEBUG;web=WARNING",
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
	srv = newTCPServer(cfg)
	srv.broadcast = hub.broadcast

	// START SERVICES

	logger.Infof("Creating SIP Connection pool with size: %v", cfg.NumSIPConnections)
	sipPool = NewSIPConnPool(cfg.NumSIPConnections)

	logger.Infof("Starting TCP server, listening at port %v", cfg.TCPPort)
	go srv.run()

	logger.Infof("Starting Websocket hub")
	go hub.run()

	http.HandleFunc("/", testHandler)
	http.HandleFunc("/ui", uiHandler)
	http.HandleFunc("/ws", wsHandler)

	logger.Infof("Starting HTTP server, listening at port %v", cfg.HTTPPort)
	http.ListenAndServe(":"+cfg.HTTPPort, nil)
}
