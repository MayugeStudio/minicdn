package main

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

var counter uint64

// ETag test content
var (
	contentMu sync.RWMutex
	content   = "initial content"
)

func main() {
	mux := http.NewServeMux()

	// Cache-Control test endpoints
	mux.HandleFunc("/max-age", cacheHandler("public, max-age=10"))
	mux.HandleFunc("/no-store", cacheHandler("no-store"))
	mux.HandleFunc("/no-cache", cacheHandler("no-cache"))
	mux.HandleFunc("/private", cacheHandler("private"))
	mux.HandleFunc("/must-revalidate", cacheHandler("public, max-age=5, must-revalidate"))

	// ETag endpoint
	mux.HandleFunc("/etag", etagHandler)

	// Update ETag content
	mux.HandleFunc("/update", updateHandler)

	// Static file server
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

func etagHandler(w http.ResponseWriter, r *http.Request) {
	contentMu.RLock()
	currentContent := content
	contentMu.RUnlock()

	hash := sha1.Sum([]byte(currentContent))
	etag := `"` + hex.EncodeToString(hash[:]) + `"`

	// Revalidation
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=5")
	w.Header().Set("ETag", etag)

	resp := map[string]interface{}{
		"content": currentContent,
		"etag":    etag,
		"time":    time.Now().Format(time.RFC3339Nano),
	}

	json.NewEncoder(w).Encode(resp)
}

func updateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	contentMu.Lock()
	content = string(body)
	contentMu.Unlock()

	resp := map[string]interface{}{
		"message": "content updated",
		"content": content,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
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
