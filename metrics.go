package main

import (
	"os"
	"time"

	"github.com/rcrowley/go-metrics"
)

type appMetrics struct {
	StartTime        time.Time
	PID              int
	ClientsConnected metrics.Counter
}

type exportMetrics struct {
	UpTime                 string
	PID                    int
	ClientsConnected       int64
	SIPPoolCurrentCapacity int
	//SIPPoolMaxCapacity     int
}

func registerMetrics() *appMetrics {
	var m appMetrics

	m.StartTime = time.Now()
	m.PID = os.Getpid()
	m.ClientsConnected = metrics.NewCounter()
	metrics.Register("ClientsConnected", m.ClientsConnected)

	return &m
}

func (m *appMetrics) Export() *exportMetrics {
	now := time.Now()
	uptime := now.Sub(m.StartTime)

	return &exportMetrics{
		UpTime:                 uptime.String(),
		PID:                    m.PID,
		ClientsConnected:       m.ClientsConnected.Count(),
		SIPPoolCurrentCapacity: sipPool.Len(),
		//SIPPoolMaxCapacity:     sipPool.MaximumCapacity(),
	}
}
