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

	"github.com/MayugeStudio/lrucache"
)

const ORIGIN_SERVER_HOSTNAME = "origin"
const ORIGIN_PORT = "9000"

const EDGE_ADDRESS = "0.0.0.0"
const EDGE_PORT = ":5000"

type Edge struct {
	rp    *httputil.ReverseProxy
	cache *lrucache.LRUCache
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
			log.Printf("Cache HIT: %s is in the cache\n", cacheKey)
			io.WriteString(w, content)
			return
		}
		log.Printf("Cache MISS: %s is not in the cache\n", cacheKey)
		next.ServeHTTP(w, r)
	})
}

func NewEdge(cacheSize int, target *url.URL) *Edge {
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
		// net/http/httputil/reverseproxy.go:164-166
		// ModifyResponse is an optional function that modifies the
		// Response from the backend. It is called if the backend
		// returns a response at all, with any HTTP status code.
		// とあるので、エラーの可能性があるため、まずはそれを確認する。
		if resp.StatusCode < 100 || resp.StatusCode > 199 {
			return fmt.Errorf("bad status code error %d", resp.StatusCode)
		}

		req := resp.Request

		// キャッシュを保存する
		cacheKey := GenerateCacheKey(req.Host, req.URL.Path, req.URL.Query())

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("request.Body is closed before it is read: %v", err)
		}
		// NOTE: もしかしたら、[]byte型がキャッシュの型として一番いいかもしれない。
		// 特に、中身をみたいわけではないし。
		edge.cache.Put(cacheKey, string(body))
		log.Printf("Add %s to cache\n", cacheKey)

		// io.ReadAllがBodyを全て読むので、新しくBodyを作成する
		resp.Body = io.NopCloser(bytes.NewReader(body))

		// Bodyを書き換える場合はContent-Lengthを更新する必要があるため必要。
		// resp.ContentLength = int64(len(body))
		// resp.Header.Set("Content-Length", strconv.Itoa(len(body)))

		return nil
	}

	edge.rp.Rewrite = rewrite
	edge.rp.ModifyResponse = modifyResponse
	edge.cache = lrucache.New(cacheSize)

	return edge
}

func main() {
	edgeNumber, ok := os.LookupEnv("EDGE_NUMBER")
	if !ok {
		panic("Failed to get EDGE_NUMBER")
	}
	log.SetPrefix(fmt.Sprintf("[EDGE%s] ", edgeNumber))

	target, err := url.Parse("http://" + ORIGIN_SERVER_HOSTNAME + ":9000")
	if err != nil {
		panic("Failed to parse the url of the origin server")
	}

	edge := NewEdge(8, target) // NOTE: 8 is too small for the cache size
	server := http.Server{
		Addr:    EDGE_ADDRESS + EDGE_PORT,
		Handler: logger(checkCache(edge.rp, edge)),
	}

	hostname, err := os.Hostname()
	if err != nil {
		panic(err)
	}

	log.Printf("Start Listening at %s%s\n", hostname, EDGE_PORT)
	log.Fatalln(server.ListenAndServe())
}
