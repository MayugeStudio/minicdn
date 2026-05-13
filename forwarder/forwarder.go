package main

import (
	"log"
	"net/url"
	"net/http"
	"net/http/httputil"
	"os"

	"github.com/MayugeStudio/minicdn/forwarder/maglev"
)

const FORWARDER_PORT = ":8000"

func reverseProxy(target *url.URL) *httputil.ReverseProxy {
	rp := &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			r.SetURL(target)
			r.Out.Host = r.In.Host
		},
	}

	return rp
}

func parseURL(rawURL string) *url.URL{
	u, err := url.Parse(rawURL)
	if err != nil {
		log.Fatalf("failed to parse url %s\n", rawURL)
	}
	return u
}

type Forwarder struct {
	ml 			 *maglev.Maglev
	proxies  map[string]*httputil.ReverseProxy
	backends []*Backend
}

func New(backends []*Backend) *Forwarder {
	n := len(backends)
	names := make([]string, n)
	for i := 0; i < n; i++ {
		names[i] = backends[i].Name
	}

	proxies := make(map[string]*httputil.ReverseProxy)
	for _, backend := range backends {
		proxies[backend.Name] = reverseProxy(backend.Addr)
	}

	forwarder := &Forwarder{
		proxies: proxies,
		backends: backends,
		ml: maglev.New(names, maglev.SmallM),
	}

	return forwarder
}

func (f *Forwarder) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Path
	backend := f.ml.Lookup(key)

	f.proxies[backend].ServeHTTP(w, r)
}


func main() {
	backends := []*Backend{
		&Backend{
			Name: "cachenode01",
			Addr: parseURL("http://edge1:5000"),
		},
		&Backend{
			Name: "cachenode02",
			Addr: parseURL("http://edge2:5000"),
		},
	}
	f := New(backends)
	server := http.Server{
		Addr: "0.0.0.0:8000",
		Handler: f,
	}

	hostname, err := os.Hostname()
	if err != nil {
		panic(err)
	}

	log.Printf("Start Listening at %s%s\n", hostname, FORWARDER_PORT)
	log.Fatalln(server.ListenAndServe())
}

