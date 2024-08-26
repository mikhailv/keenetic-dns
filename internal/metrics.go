package internal

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	promNamespace = "keenetic_dns"
)

var (
	durationBuckets = []float64{0.001, 0.002, 0.005, 0.01, 0.02, 0.05, 0.1, 0.2, 0.5, 1, 2, 5, 10, 20, 60, 120, 600} // 17 items

	operationDurationHistogram = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: promNamespace,
		Name:      "operation_duration_seconds",
		Buckets:   durationBuckets,
	}, []string{"operation"})

	operationStatusCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: promNamespace,
		Name:      "operation_status",
	}, []string{"operation", "status"})
)

func TrackDuration(operation string) func() {
	start := time.Now()
	return func() {
		operationDurationHistogram.WithLabelValues(operation).Observe(time.Since(start).Seconds())
	}
}

func TrackStatus(operation, status string) {
	operationStatusCounter.WithLabelValues(operation, status).Inc()
}
