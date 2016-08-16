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
	sipIDs  *sipID
	hub     *Hub
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
			SIPServer:         "localhost:6001",
			SIPUser:           "autouser",
			SIPPass:           "autopass",
			NumSIPConnections: 3,
		}
		log.Printf("ERROR: Couldn't read config file: %v", err.Error())
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

	log.Printf("Config: %+v", cfg)

	hub = newHub()
	status = registerMetrics()

	// START SERVICES
	sipIDs = newSipIDs(cfg.NumSIPConnections)
	log.Printf("Creating SIP Connection pool with size: %v", cfg.NumSIPConnections)
	sipPool, err = pool.NewChannelPool(0, cfg.NumSIPConnections, initSIPConn)
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
