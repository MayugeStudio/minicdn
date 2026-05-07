package main

import (
	"io"
	"log"
	"net/http"
)

const hostname = "origin"

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[%s] receive request %s\n", r.Method, r.RequestURI)
		io.WriteString(w, "Hello from Origin Server")
	})

	log.Printf("[Origin] Start Listening at %s:9000", hostname)
	log.Fatalln(http.ListenAndServe(":9000", nil))
}
