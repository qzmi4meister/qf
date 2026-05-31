package api

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func handleMetrics(w http.ResponseWriter, r *http.Request) {
	promhttp.Handler().ServeHTTP(w, r)
}
