package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"maps"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/hashicorp/golang-lru/v2/expirable"
)

const ORIGIN_SERVER_HOSTNAME = "origin"
const ORIGIN_PORT = "9000"

const EDGE_ADDRESS = "0.0.0.0"
const EDGE_PORT = ":5000"

const HEALTH_CHECK_PORT = ":3232"

// const TTL = time.Minute * 5
const TTL = time.Minute * 60

type Edge struct {
	rp    *httputil.ReverseProxy
	cache *expirable.LRU[string, string]
}

func (e *Edge) HandleHealthCheck(w http.ResponseWriter, r *http.Request) {
	log.Printf("Received health check request from %s", r.RemoteAddr)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
}

// GenerateCacheKeyはキャッシュのキーをホスト、パス、クエリから作成する。
// 作成する際、クエリパラメータの順番は無視する。
// ホスト、パス、クエリの例: http://127.0.0.1:5001/index.html?test=hoge
// ホスト: 127.0.0.1
// パス  : /index.html
// クエリ: ?test=hoge
func GenerateCacheKey(host string, path string, queries url.Values) string {
	// sha256関数が[]byteを受け取るので、[]byteにhost, path, queriesを変換する
	byteArray := make([]byte, len(host)+len(path)+len(queries)) // 正確ではないけど、一旦確保

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
		cacheKey := GenerateCacheKey(r.Host, r.URL.Path, r.URL.Query())

		// キャッシュキーが存在するかチェックする
		if content, ok := e.cache.Get(cacheKey); ok {
			log.Printf("Cache HIT: %.8s is in the cache\n", cacheKey)
			io.WriteString(w, content)
			return
		}
		log.Printf("Cache MISS: %.8s is not in the cache\n", cacheKey)
		next.ServeHTTP(w, r)
	})
}

func NewEdge(cacheSize int, ttl time.Duration, target *url.URL) *Edge {
	edge := &Edge{
		rp: &httputil.ReverseProxy{},
	}

	// rewriteは、レスポンスをオリジンサーバに転送する
	rewrite := func(pr *httputil.ProxyRequest) {
		// SetURLの中身
		// rewriteRequestURL(r.Out, target)
		// r.Out.Host = ""
		pr.SetURL(target)

		pr.Out.Host = pr.In.Host
		pr.Out.Header["X-Forwarded-For"] = pr.In.Header["X-Forwarded-For"]

		// X-Forwarded-XXXX系のヘッダを書き込む
		pr.SetXForwarded()
	}

	// modifyResponseは、Responseを受け取った後に呼ばれるので、キャッシュの保存に利用する。
	modifyResponse := func(resp *http.Response) error {

		req := resp.Request

		// キャッシュを保存する
		cacheKey := GenerateCacheKey(req.Host, req.URL.Path, req.URL.Query())

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("request.Body is closed before it is read: %v", err)
		}
		// NOTE: もしかしたら、[]byte型がキャッシュの型として一番いいかもしれない。
		// 特に、中身をみたいわけではないし。
		edge.cache.Add(cacheKey, string(body))
		log.Printf("Add %.8s to cache\n", cacheKey)

		// io.ReadAllがBodyを全て読むので、新しくBodyを作成する
		resp.Body = io.NopCloser(bytes.NewReader(body))

		return nil
	}

	edge.rp.Rewrite = rewrite
	edge.rp.ModifyResponse = modifyResponse
	onEvict := func (key string, value string) {
		log.Printf("%.8s has been evicted", key)
	}
	edge.cache = expirable.NewLRU[string, string](cacheSize, onEvict, ttl)

	return edge
}

func main() {
	// ログの設定
	edgeNumber, ok := os.LookupEnv("EDGE_NUMBER")
	if !ok {
		panic("Failed to get EDGE_NUMBER")
	}
	log.SetPrefix(fmt.Sprintf("[EDGE%s] ", edgeNumber))


	// データ用ポート
	target, err := url.Parse("http://" + ORIGIN_SERVER_HOSTNAME + ":9000")
	if err != nil {
		panic("Failed to parse the url of the origin server")
	}

	edge := NewEdge(8, TTL, target) // NOTE: 8 is too small for the cache size
	server := http.Server{
		Addr:    EDGE_ADDRESS + EDGE_PORT,
		Handler: logger(checkCache(edge.rp, edge)),
	}

	hostname, err := os.Hostname()
	if err != nil {
		panic(err)
	}

	log.Printf("Data server: Start listening at %s%s\n", hostname, EDGE_PORT)
	go server.ListenAndServe()

	// ヘルスチェック用ポート
	healthCheckMux := http.NewServeMux()
	healthCheckMux.HandleFunc("/health", edge.HandleHealthCheck)

	healthCheckServer := &http.Server{
		Addr: "0.0.0.0" + HEALTH_CHECK_PORT,
		Handler: healthCheckMux,
	}

	log.Printf("Control server: Start listening at %s%s\n", hostname, HEALTH_CHECK_PORT)
	healthCheckServer.ListenAndServe()
}

