// Command dashboard serves a simple UI that calls the analytics API and shows charts.
package main

import (
	"embed"
	"flag"
	"io/fs"
	"log"
	"net/http"
	"os"
)

//go:embed static
var staticFS embed.FS

func main() {
	addr := flag.String("addr", ":8081", "Listen address for dashboard")
	apiBase := flag.String("api", "http://localhost:8080", "Analytics API base URL (or DASHBOARD_API env)")
	flag.Parse()

	if v := os.Getenv("DASHBOARD_API"); v != "" && *apiBase == "http://localhost:8080" {
		*apiBase = v
	}

	strip, _ := fs.Sub(staticFS, "static")
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		index, _ := fs.ReadFile(strip, "index.html")
		body := bytesReplace(index, []byte("__API_BASE__"), []byte(*apiBase))
		w.Write(body)
	})

	log.Printf("dashboard listening on %s (api=%s)", *addr, *apiBase)
	log.Fatal(http.ListenAndServe(*addr, mux))
}

func bytesReplace(b, old, new []byte) []byte {
	out := make([]byte, 0, len(b))
	for {
		i := indexBytes(b, old)
		if i < 0 {
			out = append(out, b...)
			return out
		}
		out = append(out, b[:i]...)
		out = append(out, new...)
		b = b[i+len(old):]
	}
}

func indexBytes(b, sep []byte) int {
	for i := 0; i <= len(b)-len(sep); i++ {
		if equal(b[i:i+len(sep)], sep) {
			return i
		}
	}
	return -1
}

func equal(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
