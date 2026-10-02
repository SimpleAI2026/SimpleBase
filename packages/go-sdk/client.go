// Package gosdk provides a standard-library client for SimpleBase's project API.
package gosdk

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
)

// Options configures a project-scoped client. HTTPClient defaults to http.DefaultClient.
type Options struct {
	URL        string
	APIKey     string
	ProjectID  string
	DatabaseID string
	HTTPClient *http.Client
}

// Client is safe to share between goroutines when its HTTP client is safe to share.
type Client struct {
	baseURL    string
	apiKey     string
	projectID  string
	databaseID string
	httpClient *http.Client
}

// NewClient validates the endpoint and credentials without contacting the server.
func NewClient(opts Options) (*Client, error) {
	if strings.TrimSpace(opts.APIKey) == "" || strings.TrimSpace(opts.ProjectID) == "" {
		return nil, errors.New("gosdk: APIKey and ProjectID are required")
	}
	u, err := url.Parse(opts.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return nil, errors.New("gosdk: URL must be an HTTP(S) origin")
	}
	hc := opts.HTTPClient
	if hc == nil {
		hc = http.DefaultClient
	}
	return &Client{baseURL: strings.TrimRight(u.String(), "/"), apiKey: opts.APIKey, projectID: opts.ProjectID, databaseID: opts.DatabaseID, httpClient: hc}, nil
}

// Database returns a copy of the client scoped to the specified database.
func (c *Client) Database(id string) *Client {
	copy := *c
	copy.databaseID = id
	return &copy
}

func (c *Client) projectPath(suffix string) string {
	return "/v1/projects/" + url.PathEscape(c.projectID) + suffix
}

func (c *Client) databasePath(suffix string) (string, error) {
	if strings.TrimSpace(c.databaseID) == "" {
		return "", errors.New("gosdk: database ID is required; set Options.DatabaseID or call Database")
	}
	return c.projectPath("/databases/" + url.PathEscape(c.databaseID) + suffix), nil
}
