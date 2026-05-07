package main

import (
	"log"
	"os"
	"net/http"
	"net/http/httputil"
)

const ORIGIN_SERVER_HOSTNAME = "origin"

func main() {
	rewrite := func(pr *httputil.ProxyRequest) {
		pr.Out.URL.Scheme = "http"
		pr.Out.URL.Host = ORIGIN_SERVER_HOSTNAME + ":9000"
		log.Println(pr.Out.URL.Host)
	}

	rp := &httputil.ReverseProxy{
		Rewrite: rewrite,
	}

	server := http.Server{
		Addr: "0.0.0.0:5000",
		Handler: rp,
	}

	hostname, err := os.Hostname()
	if err != nil {
		panic(err)
	}
	nodeNumber, ok := os.LookupEnv("NODE_NUMBER")
	if !ok {
		panic("Failed to get NODE_NUMBER")
	}

	log.Printf("[Node%s] Start Listening at %s:9000\n", nodeNumber, hostname)
	log.Fatalln(server.ListenAndServe())
}
