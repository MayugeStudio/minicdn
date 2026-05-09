package main

import (
	"log"
	"fmt"
	"os"
	"net/url"
	"net/http"
	"net/http/httputil"

	"github.com/MayugeStudio/lrucache"
)

type Edge struct {
	 rp			*httputil.ReverseProxy
	 cache  lrucache.LRUCache	
}

// logger
func logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Println(r.Method, r.URL.Path)
		next.ServeHTTP(w, r)
	})
}

func NewEdge(target *url.URL) *Edge {
	// Define rewrite function
	rewrite := func(pr *httputil.ProxyRequest) {
		// SetURL do the following things
		// rewriteRequestURL(r.Out, target)
		// r.Out.Host = ""

		pr.SetURL(target)
		pr.Out.Host = pr.In.Host

		pr.Out.Header["X-Forwarded-For"] = pr.In.Header["X-Forwarded-For"]
		pr.SetXForwarded()

		log.Printf("%s => %s\n", pr.In.RemoteAddr, target)
	}

	return &Edge{
		rp: &httputil.ReverseProxy{
			Rewrite: rewrite,
		},
	}
}

func (n *Edge) Start(addr string) {
	server := http.Server{
		Addr: addr,
		Handler: logger(n.rp),
	}

	hostname, err := os.Hostname()
	if err != nil {
		panic(err)
	}

	log.Printf("Start Listening at %s:9000\n", hostname)
	log.Fatalln(server.ListenAndServe())
}

const ORIGIN_SERVER_HOSTNAME = "origin"

func main() {
	edgeNumber, ok := os.LookupEnv("EDGE_NUMBER")
	if !ok {
		panic("Failed to get EDGE_NUMBER")
	}

	log.SetPrefix(fmt.Sprintf("[EDGE%s] ", edgeNumber))

	target, err := url.Parse("http://" + ORIGIN_SERVER_HOSTNAME + ":9000")
	if err != nil {
		panic("Failed to parse Origin server")
	}

	edge := NewEdge(target)
	edge.Start("0.0.0.0:5000")
}

