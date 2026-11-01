package main

import (
	"encoding/json"
	"net/http"
	"runtime"
	"time"
)

// Handler est le point d'entree Fission exporte
func Handler(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		name = "Nexterway"
	}

	response := map[string]interface{}{
		"message":   "Hello " + name + " from Go on Nexterway FaaS!",
		"status":    "healthy",
		"runtime":   "golang-" + runtime.Version(),
		"arch":      runtime.GOARCH,
		"num_cpu":   runtime.NumCPU(),
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(response)
}
