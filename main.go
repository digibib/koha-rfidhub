package main

import (
	"html/template"
	"log"
	"net/http"

	_ "net/http/pprof"
)

// APPLICATION STATE

var (
	cfg       *config
	srv       *TCPServer
	uiHub     *wsHub
	templates = template.Must(template.ParseFiles("index.html"))
)

// SETUP

func init() {
	err := cfg.fromFile("config.json")
	if err != nil {
		cfg = &config{
			TCPPort:  "6767",
			HTTPPort: "8899",
		}
		log.Printf("INFO No config file found, using standard values")
	}

	uiHub = newHub()
	srv = newTCPServer(cfg)
	srv.broadcast = uiHub.broadcast
}

// APPLICATION ENTRY POINT

func main() {
	log.Println("INFO", "Starting TCP server, listening at port", cfg.TCPPort)
	go srv.run()

	log.Println("INFO", "Starting Websocket hub")
	go uiHub.run()

	http.HandleFunc("/", testHandler)
	http.HandleFunc("/ws", wsHandler)

	log.Println("INFO", "Starting HTTP server, listening at port", cfg.HTTPPort)
	log.Fatal(http.ListenAndServe(":"+cfg.HTTPPort, nil))
}
