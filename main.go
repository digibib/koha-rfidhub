package main

import (
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"strconv"

	pool "gopkg.in/fatih/pool.v2"
)

// APPLICATION GLOBALS

var (
	cfg     = &config{}
	sipPool pool.Pool
	hub     *Hub
	status  *appMetrics
)

// APPLICATION ENTRY POINT

func main() {
	// Config defaults
	cfg = &config{
		TCPPort:           "6005",
		HTTPPort:          "8899",
		SIPServer:         "localhost:6001",
		SIPUser:           "autouser",
		SIPPass:           "autopass",
		NumSIPConnections: 3,
	}
	// Override with environment vars
	if os.Getenv("TCP_PORT") != "" {
		cfg.TCPPort = os.Getenv("TCP_PORT")
	}
	if os.Getenv("HTTP_PORT") != "" {
		cfg.HTTPPort = os.Getenv("HTTP_PORT")
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

	log.Printf("Config: %+v", cfg)

	hub = newHub()
	status = registerMetrics()

	// START SERVICES
	log.Printf("Creating SIP Connection pool with size: %v", cfg.NumSIPConnections)
	var err error
	sipPool, err = pool.NewChannelPool(0, cfg.NumSIPConnections, initSIPConn(cfg.TCPPort))
	if err != nil {
		log.Println("ERROR", err.Error())
		os.Exit(1)
	}

	log.Println("Starting Websocket hub")
	go hub.run()

	http.HandleFunc("/.status", statusHandler)
	http.HandleFunc("/ws", wsHandler)

	log.Printf("Starting HTTP server, listening at port %v", cfg.HTTPPort)
	http.ListenAndServe(":"+cfg.HTTPPort, nil)
}
