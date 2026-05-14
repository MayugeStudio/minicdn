package main

import (
	"encoding/json"
	"log"
	"net/http"
	"sync/atomic"
	"time"
)

var counter uint64

func main() {
	mux := http.NewServeMux()

	// Cache-Control test endpoints
	mux.HandleFunc("/max-age", cacheHandler("public, max-age=10"))
	mux.HandleFunc("/no-store", cacheHandler("no-store"))
	mux.HandleFunc("/no-cache", cacheHandler("no-cache"))
	mux.HandleFunc("/private", cacheHandler("private"))
	mux.HandleFunc("/must-revalidate", cacheHandler("public, max-age=5, must-revalidate"))

	// Static file server
	//
	// Put files under:
	// ./static/
	//
	// Example:
	// ./static/image.png
	//
	// Access:
	// http://localhost:9000/static/image.png
	mux.Handle("/static/", http.StripPrefix(
		"/static/",
		http.FileServer(http.Dir("./static")),
	))

	log.Println("listening on :9000")

	http.ListenAndServe(":9000", logging(mux))
}

func cacheHandler(cacheControl string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddUint64(&counter, 1)

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", cacheControl)

		resp := map[string]interface{}{
			"path":          r.URL.Path,
			"cache_control": cacheControl,
			"request_count": count,
			"time":          time.Now().Format(time.RFC3339Nano),
		}

		json.NewEncoder(w).Encode(resp)
	}
}

func logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf(
			"%s %s",
			r.Method,
			r.URL.Path,
		)

		next.ServeHTTP(w, r)
	})
}
