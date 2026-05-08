package main

import (
	"io"
	"log"
	"net/http"
)

const hostname = "origin"

func main() {
	log.SetPrefix("[ORIGIN] ")

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Got a request\n")
		log.Printf("	Sender IP (Previous Hop IP): %s\n", r.RemoteAddr)
		log.Printf("	X-Forwarded-For: %s\n", r.Header.Get("X-Forwarded-For"))

		io.WriteString(w, "Hello from Origin Server\n")
		io.WriteString(w, "I can serve my html very well\n")
	})

	log.Printf("Start Listening at %s:9000", hostname)
	log.Fatalln(http.ListenAndServe(":9000", nil))
}
