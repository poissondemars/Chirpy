package main

import (
	"fmt"
	"log"
	"net/http"
	"sync/atomic"
)

type apiConfig struct {
	fileserverHits atomic.Int32
}

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request){
		cfg.fileserverHits.Add(1)
		next.ServeHTTP(w, r)
	})
}

func (cfg *apiConfig)handleMetrics(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte(fmt.Sprintf("Hits: %d", cfg.fileserverHits.Load())))
}

func (cfg *apiConfig)handleResetMetrics(w http.ResponseWriter, r *http.Request) {
	cfg.fileserverHits.Store(0)
}

func main() {
	mux := http.NewServeMux()
	apiConfig := &apiConfig{}

	mux.Handle("/app/", middlewareLog(apiConfig.middlewareMetricsInc(http.FileServer(http.Dir(".")))))
	mux.Handle("GET /metrics", middlewareLog(http.HandlerFunc(apiConfig.handleMetrics)))
	mux.Handle("POST /reset", middlewareLog(http.HandlerFunc(apiConfig.handleResetMetrics)))
	mux.Handle("GET /healthz", middlewareLog(http.HandlerFunc(handleHealthz)))

	server := &http.Server{
		Addr: 		":8080",
		Handler: 	mux,
	}

	server.ListenAndServe()
}

func handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Content-Type", "text/plain; charset=utf-8")

	w.WriteHeader(200)
	w.Write([]byte(`OK`))
}

func middlewareLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.URL.Path)
		next.ServeHTTP(w, r)
	})
}