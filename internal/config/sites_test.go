package config

import (
	"strings"
	"testing"
)

func TestSitesHostLabelAndURL(t *testing.T) {
	s := SitesConfig{BaseDomain: "SimpleBase.top.", URLScheme: "https"}
	cases := map[string]string{
		"abcd1234.simplebase.top":     "abcd1234",
		"ABCD1234.SimpleBase.top:443": "abcd1234",
		"abcd1234.simplebase.top.":    "abcd1234",
		"a.abcd1234.simplebase.top":   "",
		"simplebase.top":              "",
		"short.simplebase.top":        "",
		"-bcd1234.simplebase.top":     "",
		"abcd1234.evil.top":           "",
	}
	for host, want := range cases {
		got, ok := s.SiteHostLabel(host)
		if got != want || ok != (want != "") {
			t.Errorf("%s: got %q %v", host, got, ok)
		}
	}
	if u := s.SubdomainBaseURL("abcd1234"); u != "https://abcd1234.simplebase.top" {
		t.Fatalf("url=%s", u)
	}
	s.URLScheme, s.URLPort, s.BaseDomain = "http", 8080, "localhost"
	if u := s.SubdomainBaseURL("abcd1234"); u != "http://abcd1234.localhost:8080" {
		t.Fatalf("url=%s", u)
	}
	s.BaseDomain = ""
	if _, ok := s.SiteHostLabel("abcd1234.localhost"); ok || s.SubdomainBaseURL("abcd1234") != "" {
		t.Fatal("unconfigured domain must not resolve")
	}
}

func TestProjectIDPublishable(t *testing.T) {
	for id, ok := range map[string]bool{"abcd1234": true, "ab-d1234": true, "ABCD1234": false, "-bcd1234": false, "abcd123-": false, "abc": false, "sb-admin": true} {
		if (ProjectIDPublishable(id) == "") != ok {
			t.Errorf("%s: want %v", id, ok)
		}
	}
}

func TestValidateSites(t *testing.T) {
	if errs := validateSites(SitesConfig{}); len(errs) != 0 {
		t.Fatal("unconfigured domain must not validate")
	}
	errs := validateSites(SitesConfig{BaseDomain: "simplebase.top", URLScheme: "ftp", URLPort: -1, IndexDocument: "../x", CacheTTL: -1})
	if len(errs) != 4 {
		t.Fatalf("errs=%v", errs)
	}
	if errs := validateSites(SitesConfig{BaseDomain: "*.simplebase.top"}); len(errs) != 1 {
		t.Fatalf("wildcard: %v", errs)
	}
	if errs := validateSites(SitesConfig{BaseDomain: "simplebase.top"}); len(errs) != 0 {
		t.Fatalf("ok: %v", errs)
	}
	s := SitesConfig{}
	if s.EffectiveIndexDocument() != "index.html" || s.EffectiveCacheTTL() != DefaultSiteCacheTTL {
		t.Fatal("defaults")
	}
}

func TestSitesSameSiteWarning(t *testing.T) {
	c := Config{Sites: SitesConfig{BaseDomain: "simplebase.top"}}
	c.HTTP.PublicURL = "https://console.simplebase.top"
	if !strings.Contains(SitesSameSiteWarning(c), "same-site") {
		t.Fatal("want warning")
	}
	c.HTTP.PublicURL = "https://console.example.com"
	if SitesSameSiteWarning(c) != "" {
		t.Fatal("no warning expected")
	}
}

func TestSitesEnvAndYAML(t *testing.T) {
	t.Setenv("SIMPLEBASE_SITES_BASE_DOMAIN", "localhost")
	t.Setenv("SIMPLEBASE_SITES_URL_SCHEME", "http")
	t.Setenv("SIMPLEBASE_SITES_URL_PORT", "8080")
	t.Setenv("SIMPLEBASE_SITES_NOT_FOUND_DOCUMENT", "404.html")
	cfg := loadFromEnv()
	if cfg.Sites.BaseDomain != "localhost" || cfg.Sites.URLPort != 8080 || cfg.Sites.NotFoundDocument != "404.html" {
		t.Fatalf("env: %+v", cfg.Sites)
	}
	yc, err := unmarshalYAML([]byte("sites:\n  base_domain: simplebase.top\n  cache_ttl: 5s\n  csp: \"off\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	cfg = defaults()
	if _, err := applyYAML(&cfg, yc); err != nil {
		t.Fatal(err)
	}
	if cfg.Sites.BaseDomain != "simplebase.top" || cfg.Sites.CacheTTL.String() != "5s" || cfg.Sites.CSP != "off" {
		t.Fatalf("yaml: %+v", cfg.Sites)
	}
}

func TestSitesModeAndPathURL(t *testing.T) {
	if (SitesConfig{}).Mode() != SiteModePath || (SitesConfig{Subdomain: true}).Mode() != SiteModeSubdomain {
		t.Fatal("mode")
	}
	for in, want := range map[string]string{"": "/sites", "/": "/sites", "sites": "/sites", "/pub/": "/pub", " /x ": "/x"} {
		if got := (SitesConfig{PathPrefix: in}).EffectivePathPrefix(); got != want {
			t.Errorf("prefix %q: got %q want %q", in, got, want)
		}
	}
	s := SitesConfig{}
	if u := s.PathBaseURL("https://api.example.com/", "abcd1234"); u != "https://api.example.com/sites/abcd1234" {
		t.Fatalf("url=%s", u)
	}
	if u := s.PathBaseURL("http://h:8080", "AB cd/12"); u != "http://h:8080/sites/AB%20cd%2F12" {
		t.Fatalf("escape url=%s", u)
	}
}

func TestSitesEnvSubdomain(t *testing.T) {
	for v, want := range map[string]bool{"1": true, "true": true, "0": false, "false": false, "garbage": false} {
		t.Setenv("SIMPLEBASE_SITES_SUBDOMAIN", v)
		if got := loadFromEnv().Sites.Subdomain; got != want {
			t.Errorf("%q: got %v want %v", v, got, want)
		}
	}
	t.Setenv("SIMPLEBASE_SITES_PATH_PREFIX", "/pub")
	t.Setenv("SIMPLEBASE_SITES_PUBLIC_BASE_URL", "https://api.example.com")
	cfg := loadFromEnv()
	if cfg.Sites.PathPrefix != "/pub" || cfg.Sites.PublicBaseURL != "https://api.example.com" {
		t.Fatalf("env: %+v", cfg.Sites)
	}
	t.Setenv("SIMPLEBASE_AGENT_MCP_BUILTIN", "false")
	if loadFromEnv().Agent.MCP.Builtin {
		t.Fatal("mcp builtin env")
	}
	if !defaults().Agent.MCP.Builtin {
		t.Fatal("mcp builtin default must be true")
	}
}

func TestValidateSitesPathMode(t *testing.T) {
	cases := []struct {
		name string
		s    SitesConfig
		bad  bool
	}{
		{"default", SitesConfig{}, false},
		{"custom prefix", SitesConfig{PathPrefix: "/pub_1"}, false},
		{"reserved v1", SitesConfig{PathPrefix: "/v1"}, true},
		{"reserved go", SitesConfig{PathPrefix: "go"}, true},
		{"reserved __site", SitesConfig{PathPrefix: "/__site"}, true},
		{"multi segment", SitesConfig{PathPrefix: "/a/b"}, true},
		{"upper", SitesConfig{PathPrefix: "/Sites"}, true},
		{"csp off path", SitesConfig{CSP: "off"}, true},
		{"csp off subdomain", SitesConfig{Subdomain: true, BaseDomain: "x.top", CSP: "off"}, false},
		{"base url ok", SitesConfig{PublicBaseURL: "https://api.example.com/base"}, false},
		{"base url no scheme", SitesConfig{PublicBaseURL: "api.example.com"}, true},
		{"base url query", SitesConfig{PublicBaseURL: "https://a.com/?x=1"}, true},
		{"base url fragment", SitesConfig{PublicBaseURL: "https://a.com/#x"}, true},
		{"subdomain without domain", SitesConfig{Subdomain: true}, false},
	}
	for _, tc := range cases {
		if errs := validateSites(tc.s); (len(errs) > 0) != tc.bad {
			t.Errorf("%s: errs=%v bad=%v", tc.name, errs, tc.bad)
		}
	}
}
