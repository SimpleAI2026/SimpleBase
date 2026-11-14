package config

import (
	"strings"
	"testing"
)

func TestSitesHostLabelAndURL(t *testing.T) {
	s := SitesConfig{Enabled: true, BaseDomain: "SimpleBase.top.", URLScheme: "https"}
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
	if u := s.SiteBaseURL("abcd1234"); u != "https://abcd1234.simplebase.top" {
		t.Fatalf("url=%s", u)
	}
	s.URLScheme, s.URLPort, s.BaseDomain = "http", 8080, "localhost"
	if u := s.SiteBaseURL("abcd1234"); u != "http://abcd1234.localhost:8080" {
		t.Fatalf("url=%s", u)
	}
	s.Enabled = false
	if _, ok := s.SiteHostLabel("abcd1234.localhost"); ok || s.SiteBaseURL("abcd1234") != "" {
		t.Fatal("disabled must not resolve")
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
		t.Fatal("disabled must not validate")
	}
	errs := validateSites(SitesConfig{Enabled: true, URLScheme: "ftp", URLPort: -1, IndexDocument: "../x", CacheTTL: -1})
	if len(errs) != 5 {
		t.Fatalf("errs=%v", errs)
	}
	if errs := validateSites(SitesConfig{Enabled: true, BaseDomain: "*.simplebase.top"}); len(errs) != 1 {
		t.Fatalf("wildcard: %v", errs)
	}
	if errs := validateSites(SitesConfig{Enabled: true, BaseDomain: "simplebase.top"}); len(errs) != 0 {
		t.Fatalf("ok: %v", errs)
	}
	s := SitesConfig{}
	if s.EffectiveIndexDocument() != "index.html" || s.EffectiveCacheTTL() != DefaultSiteCacheTTL {
		t.Fatal("defaults")
	}
}

func TestSitesSameSiteWarning(t *testing.T) {
	c := Config{Sites: SitesConfig{Enabled: true, BaseDomain: "simplebase.top"}}
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
	t.Setenv("SIMPLEBASE_SITES_ENABLED", "true")
	t.Setenv("SIMPLEBASE_SITES_BASE_DOMAIN", "localhost")
	t.Setenv("SIMPLEBASE_SITES_URL_SCHEME", "http")
	t.Setenv("SIMPLEBASE_SITES_URL_PORT", "8080")
	t.Setenv("SIMPLEBASE_SITES_NOT_FOUND_DOCUMENT", "404.html")
	cfg := loadFromEnv()
	if !cfg.Sites.Enabled || cfg.Sites.BaseDomain != "localhost" || cfg.Sites.URLPort != 8080 || cfg.Sites.NotFoundDocument != "404.html" {
		t.Fatalf("env: %+v", cfg.Sites)
	}
	yc, err := unmarshalYAML([]byte("sites:\n  enabled: true\n  base_domain: simplebase.top\n  cache_ttl: 5s\n  csp: \"off\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	cfg = defaults()
	if _, err := applyYAML(&cfg, yc); err != nil {
		t.Fatal(err)
	}
	if !cfg.Sites.Enabled || cfg.Sites.BaseDomain != "simplebase.top" || cfg.Sites.CacheTTL.String() != "5s" || cfg.Sites.CSP != "off" {
		t.Fatalf("yaml: %+v", cfg.Sites)
	}
}
