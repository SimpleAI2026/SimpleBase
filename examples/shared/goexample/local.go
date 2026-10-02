package goexample

import (
	"encoding/json"
	"net/http"
	"net/url"
)

// LocalOnly restricts the credential-bearing demo BFF to same-host browser traffic.
func LocalOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			next.ServeHTTP(w, r)
			return
		}
		origin := r.Header.Get("Origin")
		parsed, err := url.Parse(origin)
		if err != nil || parsed.Host != r.Host || parsed.Hostname() != "127.0.0.1" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "local origin required"})
			return
		}
		next.ServeHTTP(w, r)
	})
}
