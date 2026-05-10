package main

import (
	"log"
	"fmt"
	"os"
	"slices"
	"strings"
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
func GenerateCacheKey(host string, path string, queries url.Values) string {
	// sha256関数が[]byteを受け取るので、[]byteにhost, path, queriesを変換する
	byteArray := make([]byte, len(host) + len(path) + len(queries)) // 正確ではないけど、一旦確保

	// queriesの順番を無視するための処理。
	// ?hoge=fuga&bar=baz
	// の場合、
	for _, k := range slices.Sorted(maps.Keys(queries)) {
		byteArray = append(byteArray, []byte(k)...)
		// カンマ区切りで記述されているクエリパラメータの場合も、ソートをすることによって順番の違いを無視する
		// ex. /index.html?test=1,3,2 === ./index.html?test=1,2,3
		values := queries[k][:]
		slices.Sort(values)
		byteArray = append(byteArray, []byte(strings.Join(values, ""))...)
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

// checkCacheはキャッシュがあるかどうかを確認します。また、存在した場合はキャッシュしていたボディ部分を返します。
// 同時にログを書き込みます。
func checkCache(next http.Handler, e *Edge) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// キャッシュキーを生成する
		cacheKey := GenerateCacheKey(r.URL.Host, r.URL.Path, r.URL.Query())
		// キャッシュキーが存在するかチェックする
		if content, ok := e.cache.Get(cacheKey); ok {
			log.Printf("Cache HIT: %s is in the cache\n", cacheKey)
			fmt.Fprint(w, content)
			return
		}
		log.Printf("Cache MISS: %s is not in the cache\n", cacheKey)
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
		Handler: checkCache(logger(n.rp), n),
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

