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

type Node struct {
	 rp			*httputil.ReverseProxy
	 cache  lrucache.LRUCache	
}

func NewNode(target *url.URL) *Node {
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

	return &Node{
		rp: &httputil.ReverseProxy{
			Rewrite: rewrite,
		},
	}
}

func (n *Node) Start(addr string) {
	server := http.Server{
		Addr: addr,
		Handler: n.rp,
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
	nodeNumber, ok := os.LookupEnv("NODE_NUMBER")
	if !ok {
		panic("Failed to get NODE_NUMBER")
	}

	log.SetPrefix(fmt.Sprintf("[NODE%s] ", nodeNumber))

	target, err := url.Parse("http://" + ORIGIN_SERVER_HOSTNAME + ":9000")
	if err != nil {
		panic("Failed to parse Origin server")
	}

	node := NewNode(target)
	node.Start("0.0.0.0:5000")
}

