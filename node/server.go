package main

import (
	"log"
	"fmt"
	"os"
	"net/url"
	"net/http"
	"net/http/httputil"
)

const ORIGIN_SERVER_HOSTNAME = "origin"

func main() {
	nodeNumber, ok := os.LookupEnv("NODE_NUMBER")
	if !ok {
		panic("Failed to get NODE_NUMBER")
	}
	log.SetPrefix(fmt.Sprintf("[NODE%s] ", nodeNumber))

	target, err := url.Parse("http://" + ORIGIN_SERVER_HOSTNAME + ":9000")
	if err != nil {
		panic("Failed to parse Origin server")
	}
	
	rewrite := func(pr *httputil.ProxyRequest) {
		pr.SetURL(target)
		pr.Out.Host = pr.In.Host
		pr.SetXForwarded()
		pr.Out.Header["X-Forwarded-For"] = pr.In.Header["X-Forwarded-For"]

		log.Printf("%s -> %s\n", pr.In.RemoteAddr, target)
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

	log.Printf("Start Listening at %s:9000\n", hostname)
	log.Fatalln(server.ListenAndServe())
}
