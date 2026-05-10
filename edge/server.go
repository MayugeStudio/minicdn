package main

import (
	"log"
	"fmt"
	"os"
	"slices"
	"maps"
	"net/url"
	"net/http"
	"net/http/httputil"
	"encoding/hex"
	"crypto/sha256"

	"github.com/MayugeStudio/lrucache"
)

type Edge struct {
	 rp			*httputil.ReverseProxy
	 cache  lrucache.LRUCache	
}

// GenerateCacheKeyはキャッシュのキーをホスト、パス、クエリから作成する。
// 作成する際、クエリパラメータの順番は無視する。
// ホスト、パス、クエリの例: http://127.0.0.1:5001/index.html?test=hoge
// ホスト: 127.0.0.1
// パス  : /index.html
// クエリ: ?test=hoge
func GenerateCacheKey(host string, path string, queries map[string]string) string {
	// sha256関数が[]byteを受け取るので、[]byteにhost, path, queriesを変換する
	byteArray := make([]byte, len(host) + len(path) + len(queries)) // 正確ではないけど、一旦確保

	// queriesの順番を無視するための処理。
	// ?hoge=fuga&bar=baz
	// の場合、
	for _, k := range slices.Sorted(maps.Keys(queries)) {
		byteArray = append(byteArray, []byte(k)...)
		byteArray = append(byteArray, []byte(queries[k])...)
	}

	byteArray = append(byteArray, []byte(path)...)
	byteArray = append(byteArray, []byte(host)...)

	hashValue := sha256.Sum256(byteArray)
	return hex.EncodeToString(hashValue[:])
}

// logger
func logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Println(r.Method, r.URL.Path)
		next.ServeHTTP(w, r)
	})
}

func NewEdge(target *url.URL) *Edge {
	// SetURL do the following things
	// rewriteRequestURL(r.Out, target)
	// r.Out.Host = ""

	rewrite := func(pr *httputil.ProxyRequest) {
		pr.SetURL(target)
		pr.Out.Host = pr.In.Host
		pr.Out.Header["X-Forwarded-For"] = pr.In.Header["X-Forwarded-For"]
		pr.SetXForwarded()
		log.Printf("%s => %s\n", pr.In.RemoteAddr, target)
		log.Printf("  pr.In.URL.Query() => %s\n", pr.In.URL.Query())
		log.Printf("  pr.In.URL.Path => %s\n", pr.In.URL.Path)
		log.Printf("  pr.In.Host => %s\n", pr.In.Host)
	}

	// hash := GenerateCacheKey(pr.In.Host, pr.In.URL.Path, pr.In.URL.Query())

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

