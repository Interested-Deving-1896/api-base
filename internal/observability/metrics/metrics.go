// Package metrics exposes Prometheus metrics for HTTP requests.
//
// Three metrics are collected:
//   - http_requests_total: counter, how many requests we've served
//   - http_request_duration_seconds: histogram, how long each request took
//   - http_requests_in_flight: gauge, how many requests are currently being
//     processed
//
// Call Init once at startup to register the collectors. Then use:
//   - Middleware() on every route to collect measurements
//   - Handler() on the /metrics endpoint so Prometheus can scrape it
//
// IMPORTANT: the middleware uses c.FullPath() for the path label, which
// gives us the route template (/todos/:id) rather than the actual URL
// (/todos/abc-123). If we used the URL, every unique ID would create a
// new label combination and our metric memory would explode. Never change
// this without understanding why.
package metrics

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	requestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total HTTP requests by method, path, status",
		},
		[]string{"method", "path", "status"},
	)
	requestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "Request duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)
	inFlight = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "http_requests_in_flight",
			Help: "In-flight HTTP requests",
		},
	)
)

func Init() {
	prometheus.MustRegister(requestsTotal, requestDuration, inFlight)
}

func Handler() gin.HandlerFunc {
	h := promhttp.Handler()
	return func(c *gin.Context) { h.ServeHTTP(c.Writer, c.Request) }
}

func Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		inFlight.Inc()
		defer inFlight.Dec()

		c.Next()

		path := c.FullPath()
		if path == "" {
			path = "unknown"
		}
		status := strconv.Itoa(c.Writer.Status())
		requestsTotal.WithLabelValues(c.Request.Method, path, status).Inc()
		requestDuration.WithLabelValues(c.Request.Method, path).Observe(time.Since(start).Seconds())
	}
}
