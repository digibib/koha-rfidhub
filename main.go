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
	sipPool pool.Pool // TODO move to hub struct
	hub     *Hub
	status  *appMetrics // TODO move to hub struct
)

// APPLICATION ENTRY POINT

func init() {
	status = registerMetrics()

	// TODO create struct implementing http.handler
	http.HandleFunc("/.status", statusHandler)
	http.HandleFunc("/ws", wsHandler)
}

func main() {
	// Config defaults
	cfg := config{
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

	hub = newHub(cfg)
	status = registerMetrics()

	log.Println("Starting Websocket hub")
	go hub.run()

	log.Printf("Starting HTTP server, listening at port %v", cfg.HTTPPort)
	if err := http.ListenAndServe(":"+cfg.HTTPPort, nil); err != nil {
		log.Fatal(err)
	}
}
