package main

import (
	"os"
	"time"

	"github.com/rcrowley/go-metrics"
)

type appMetrics struct {
	StartTime        time.Time
	PID              int
	ClientsKnown     int
	ClientsConnected metrics.Counter
}

type exportMetrics struct {
	UpTime           string
	PID              int
	ClientsKnown     int
	ClientsConnected int64
}

func registerMetrics() *appMetrics {
	var m appMetrics

	m.StartTime = time.Now()
	m.PID = os.Getpid()
	m.ClientsKnown = len(cfg.Clients)
	m.ClientsConnected = metrics.NewCounter()
	metrics.Register("ClientsConnected", m.ClientsConnected)

	return &m
}

func (m *appMetrics) Export() *exportMetrics {
	now := time.Now()
	uptime := now.Sub(m.StartTime)

	return &exportMetrics{
		UpTime:           uptime.String(),
		PID:              m.PID,
		ClientsKnown:     m.ClientsKnown,
		ClientsConnected: m.ClientsConnected.Count(),
	}
}
