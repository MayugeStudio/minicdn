package main

import (
	"log"
	"net/url"
	"net/http"
	"net/http/httputil"
	"os"
	"time"

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
		proxies[backend.Name] = reverseProxy(backend.DataAddr)
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
	log.Printf("Request from %s to %s is directed at %s\n", r.RemoteAddr, r.Host + r.URL.Path, backend)

	f.proxies[backend].ServeHTTP(w, r)
}

func (f *Forwarder) StartControlEndpointGoroutine(backends []*Backend) {
	log.Printf("Control endpoint: Start Health checking\n")
	ticker := time.NewTicker(time.Second * 10)
	for {
		select {
		case <-ticker.C:
			// 全てのバックエンドにヘルスチェックHTTPパケットを送信する
			for i, backend := range backends {
				ok := doHealthCheck(backend.ControlAddr)

				if ok && !backend.IsAlive { 
					// 応答していなかったはずなのに、応答するということは復活している。
					// 再び選ばれるようにする。
					f.ml.Revive(i)

					// 現在稼働中として記録しておく
					backend.IsAlive = true

					log.Printf("Health check: %s is currently running", backend.Name)
					continue
				}

				if !ok && backend.IsAlive {
					// ヘルスチェックに応答しなかった場合
					// このバックエンドがMaglev Hashingで選ばれないようにする。
					f.ml.Kill(i)

					// 現在ダウン中として記録しておく
					backend.IsAlive = false

					log.Printf("Health check: %s is currently not running\n", backend.Name)
				}
			}
		}
	}
}

func doHealthCheck(url *url.URL) bool {
	log.Printf("Health check: Send packet to %s\n", url.String())
	resp, err := http.Get(url.String())
	if err != nil {
		return false
	}

	defer resp.Body.Close()
	return true
}


func main() {
	backends := []*Backend{
		&Backend{
			Name: "cachenode01",
			DataAddr: parseURL("http://edge1:5000"),
			ControlAddr: parseURL("http://edge1:3232"),
			IsAlive: true,
		},
		&Backend{
			Name: "cachenode02",
			DataAddr: parseURL("http://edge2:5000"),
			ControlAddr: parseURL("http://edge2:3232"),
			IsAlive: true,
		},
	}


	f := New(backends)


	// ヘルスチェック用Goroutineの立ち上げ
	go f.StartControlEndpointGoroutine(backends)


	// データ転送用ポート
	server := http.Server{
		Addr: "0.0.0.0:8000",
		Handler: f,
	}

	hostname, err := os.Hostname()
	if err != nil {
		panic(err)
	}

	log.Printf("Data endpoint: Start Listening at %s%s\n", hostname, FORWARDER_PORT)
	log.Fatalln(server.ListenAndServe())
}

