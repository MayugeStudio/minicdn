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
	"strconv"
	"time"

	"github.com/hashicorp/golang-lru/v2"
)

const ORIGIN_SERVER_HOSTNAME = "origin"
const ORIGIN_PORT = "9000"

const EDGE_ADDRESS = "0.0.0.0"
const EDGE_PORT = ":5000"

const HEALTH_CHECK_PORT = ":3232"

const DEFAULT_MAXAGE = time.Second * 30

type CacheEntry struct {
	Body		 string
	ETag		 string
	StoredAt time.Time
	MaxAge   time.Duration
}

func NewCacheEntry(body string, etag string, maxAge time.Duration) *CacheEntry {
	return &CacheEntry {
		Body: 		body,
		ETag:			etag,
		StoredAt: time.Now(),
		MaxAge:   maxAge,
	}
}

func (c *CacheEntry) IsFresh() bool {
	return time.Now().Sub(c.StoredAt) <= c.MaxAge 
}

func (c *CacheEntry) HasETag() bool {
	return c.ETag != ""
}

type Edge struct {
	rp    *httputil.ReverseProxy
	cache *lru.Cache[string, *CacheEntry]
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

type CacheControl struct {
	Key   string
	Value int
}

// ParseCacheControlsはキャッシュコントロールヘッダを文字列で受け取り、
// パースして構造体に詰めた結果を返す関数。
func ParseCacheControls(rawCacheControl string) ([]CacheControl, error) {
	var res []CacheControl
	parts := strings.Split(rawCacheControl, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		// key=valueの形の時(max-age=10など)
		if strings.Contains(part, "=") {
			leftAndRight := strings.Split(part, "=")
			if len(leftAndRight) != 2 {
				return nil, fmt.Errorf("could not split %v into two values: got %v", part, leftAndRight)
			}
			left, right := leftAndRight[0], leftAndRight[1]

			converted, err := strconv.Atoi(right)
			if err != nil {
				return nil, fmt.Errorf("could not convert %v to string\n", right)
			}

			res = append(res, CacheControl{
				Key: left,
				Value: converted,
			})
		// keyのみの場合(privateなど)
		} else {
			res = append(res, CacheControl{
				Key: part,
				Value: -1,
			})
		}
	}
	return res, nil
}

type CacheConfig struct {
	MaxAge	time.Duration
	Public	bool
	Private	bool
	// NoCache bool
	NoStore	bool
}

// ConstructCacheConfigは、CacheControlの配列からCacheConfigを作成して返します。
// MaxAgeはデフォルトでDEFAULT_MAXAGEの値を使う
func ConstructCacheConfig(cacheControls []CacheControl) CacheConfig {
	var config CacheConfig
	config.MaxAge = -1
	for _, cacheControl := range cacheControls {
		switch cacheControl.Key {
		case "max-age", "s-maxage": {
			config.MaxAge = time.Second * time.Duration(cacheControl.Value)
		}
		case "public": {
			config.Public = true
		}
		case "private": {
			config.Private = true
		}
		case "no-cache": {
			config.MaxAge = 0
		}
		case "no-store": {
			config.NoStore = true
		}
		}
	}
	
	if config.MaxAge == -1 {
		config.MaxAge = DEFAULT_MAXAGE
	}

	return config
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
		if cacheEntry, ok := e.cache.Get(cacheKey); ok {
			if cacheEntry.IsFresh() {
				log.Printf("Cache HIT(fresh): %.8s is in the cache\n", cacheKey)
			} else {
				log.Printf("Cache HIT(stale): %.8s is in the cache\n", cacheKey)
			}

			if cacheEntry.IsFresh() {
				// Freshの場合、リクエストに書き込む
				io.WriteString(w, cacheEntry.Body)
				return
			}

			// Staleの場合、ETagがあれば検証
			if cacheEntry.HasETag() {
				// headerにIf-None-Matchを追加して送信。
				r.Header.Set("If-None-Match", cacheEntry.ETag)
			} else {
				// 他の場合は、普通にリクエストを投げて更新する
				log.Printf("%.8s does not have neither ETag nor Last-Modified\n", cacheKey)
			}

		} else {
			log.Printf("Cache MISS: %.8s is not in the cache\n", cacheKey)
		}
		next.ServeHTTP(w, r)
	})
}

func NewEdge(cacheSize int, target *url.URL) (*Edge, error) {
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

		// Cache-Controlヘッダを解析し、CacheConfigを作成
		var cacheConfig CacheConfig
		cacheControlHeader := resp.Header.Get("Cache-Control")
		if strings.TrimSpace(cacheControlHeader) != "" {
			log.Printf("Cache-Control(%s) exists\n", cacheControlHeader)
			cacheControls, err := ParseCacheControls(cacheControlHeader)

			if err != nil {
				log.Printf("ERROR: could not resolve cache-control headers: got %v\n", err)
			}

			// キャッシュコンフィグを解析した[]CacheControlから作成する
			cacheConfig = ConstructCacheConfig(cacheControls)

		} else {
			log.Println("Cache-Control does not exist in this request")
		}
		etag := resp.Header.Get("ETag")

		// キャッシュに保存する
		if cacheConfig.Private || cacheConfig.NoStore {
			return nil
		}

		// cacheKeyを生成
		cacheKey := GenerateCacheKey(req.Host, req.URL.Path, req.URL.Query())

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("request.Body is closed before it is read: %v", err)
		}
		// io.ReadAllがBodyを全て読むので、新しくBodyを作成する
		resp.Body = io.NopCloser(bytes.NewReader(body))

		// もし、If-None-MatchをcheckCache()でヘッダに設定していた場合
		if req.Header.Get("If-None-Match") != "" {
			cacheEntry, ok := edge.cache.Get(cacheKey)
			if !ok {
				panic("unknown error")
			}
			// 変更されていなかった場合
			if resp.StatusCode == http.StatusNotModified {
				log.Printf("%.8s is not modified\n", cacheKey)
				// storedAtを延命することによって、Freshにする
				cacheEntry.StoredAt = time.Now()
				log.Printf("	updated StoredAt\n")

			// 変更されていた場合
			} else if resp.StatusCode == http.StatusOK {
				log.Printf("%.8s is modified\n", cacheKey)
				cacheEntry.Body = string(body)
				cacheEntry.ETag = etag
				log.Printf("	updated Body and ETag\n")
			}

			return nil
		}

		cacheEntry := NewCacheEntry(string(body), etag, cacheConfig.MaxAge)
		edge.cache.Add(cacheKey, cacheEntry)
		log.Printf("Add %.8s to cache\n", cacheKey)

		return nil
	}

	edge.rp.Rewrite = rewrite
	edge.rp.ModifyResponse = modifyResponse
	cache, err := lru.New[string, *CacheEntry](cacheSize)
	if err != nil {
		return nil, fmt.Errorf("failed to create lru.Cache: %v", err)
	}
	edge.cache = cache

	return edge, nil
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

	edge, err := NewEdge(8, target) // NOTE: 8 is too small for the cache size
	if err != nil {
		log.Fatalf("failed to instantiate edge server : %v\n", err)
	}
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

