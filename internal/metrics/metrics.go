package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	ActiveSessions = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "muhan_active_sessions_total",
		Help: "The total number of active sessions",
	})
	CommandsProcessed = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "muhan_commands_processed_total",
		Help: "The total number of processed commands",
	}, []string{"command_type"})
	LoginFailures = promauto.NewCounter(prometheus.CounterOpts{
		Name: "muhan_login_failures_total",
		Help: "The total number of login failures",
	})
)
