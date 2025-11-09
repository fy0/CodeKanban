package terminal

import "github.com/prometheus/client_golang/prometheus"

type metrics struct {
	activeSessionGauge *prometheus.GaugeVec
	ioBytesCounter     *prometheus.CounterVec
}

func newMetrics() *metrics {
	m := &metrics{
		activeSessionGauge: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "kanban",
			Subsystem: "terminal",
			Name:      "sessions_active",
			Help:      "Number of active terminal sessions per project.",
		}, []string{"project_id"}),
		ioBytesCounter: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "kanban",
			Subsystem: "terminal",
			Name:      "bytes_total",
			Help:      "TTY IO throughput per project and direction.",
		}, []string{"project_id", "direction"}),
	}

	prometheus.MustRegister(m.activeSessionGauge, m.ioBytesCounter)
	return m
}
