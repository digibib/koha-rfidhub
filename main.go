package main

import (
	"html/template"
	"net/http"

	"github.com/loggo/loggo"

	_ "net/http/pprof"
)

// APPLICATION STATE

var (
	cfg        = &config{}
	srv        *TCPServer
	uiHub      *wsHub
	templates  = template.Must(template.ParseFiles("index.html", "uitest.html"))
	logger     = loggo.GetLogger("main")
	rootLogger = loggo.GetLogger("")
)

// APPLICATION ENTRY POINT

func main() {
	// SETUP
	err := cfg.fromFile("config.json")
	if err != nil {
		cfg = &config{
			TCPPort:   "6767",
			HTTPPort:  "8899",
			LogLevels: "<root>=WARNING;tcp=INFO;ws=INFO;main=INFO;sip=WARNING;rfidunit=DEBUG;web=WARNING",
		}
		logger.Warningf("No config.json file found, using standard values")
	}
	loggo.ConfigureLoggers(cfg.LogLevels)

	uiHub = newHub()
	srv = newTCPServer(cfg)
	srv.broadcast = uiHub.broadcast

	// START SERVICES

	logger.Infof("Starting TCP server, listening at port %v", cfg.TCPPort)
	go srv.run()

	logger.Infof("Starting Websocket hub")
	go uiHub.run()

	http.HandleFunc("/", testHandler)
	http.HandleFunc("/ui", uiHandler)
	http.HandleFunc("/ws", wsHandler)

	logger.Infof("Starting HTTP server, listening at port %v", cfg.HTTPPort)
	http.ListenAndServe(":"+cfg.HTTPPort, nil)
}
