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
	cfg = &config{
		TCPPort:  "6767",
		HTTPPort: "8899",
	}

	uiHub = NewHub()
	srv = newTCPServer(cfg)
	srv.broadcast = uiHub.broadcast
}

// APPLICATION ENTRY POINT

func main() {
	log.Println("INFO", "Starting TCP server")
	go srv.run()

	log.Println("INFO", "Starting Websocket hub")
	go uiHub.run()

	http.HandleFunc("/", testHandler)
	http.HandleFunc("/ws", wsHandler)

	log.Println("INFO", "Starting HTTP server")
	log.Fatal(http.ListenAndServe(":"+cfg.HTTPPort, nil))
}
