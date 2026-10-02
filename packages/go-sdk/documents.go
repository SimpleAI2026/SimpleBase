package gosdk

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"regexp"
)

var collectionNamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,62}$`)

// Document is a JSON object; numbers decode as float64 in generic values.
type Document map[string]any

// CollectionList contains names in a database.
type CollectionList struct {
	Collections []string `json:"collections"`
}

// DocumentList contains up to 1000 documents, ordered by created_at descending.
type DocumentList struct {
	Rows []Document `json:"rows"`
}

func (c *Client) collectionsPath(name string) (string, error) {
	path, err := c.databasePath("/data/collections")
	if err != nil {
		return "", err
	}
	if name == "" {
		return path, nil
	}
	if !collectionNamePattern.MatchString(name) {
		return "", errors.New("gosdk: invalid collection name")
	}
	return path + "/" + url.PathEscape(name), nil
}

// ListCollections lists document collections in the selected database.
func (c *Client) ListCollections(ctx context.Context) (*CollectionList, error) {
	path, err := c.collectionsPath("")
	if err != nil {
		return nil, err
	}
	var out CollectionList
	err = c.requestJSON(ctx, http.MethodGet, path, nil, nil, &out)
	return &out, err
}

// CreateCollection creates a collection. The success response has no body.
func (c *Client) CreateCollection(ctx context.Context, name string) error {
	if !collectionNamePattern.MatchString(name) {
		return errors.New("gosdk: invalid collection name")
	}
	path, err := c.collectionsPath("")
	if err != nil {
		return err
	}
	return c.requestJSON(ctx, http.MethodPost, path, nil, struct {
		Name string `json:"name"`
	}{name}, nil)
}

// ListDocuments lists documents in a collection.
func (c *Client) ListDocuments(ctx context.Context, collection string) (*DocumentList, error) {
	path, err := c.collectionsPath(collection)
	if err != nil {
		return nil, err
	}
	if collection == "" {
		return nil, errors.New("gosdk: collection name is required")
	}
	var out DocumentList
	err = c.requestJSON(ctx, http.MethodGet, path, nil, nil, &out)
	return &out, err
}

// InsertDocument creates a document. The server generates an ID when absent.
func (c *Client) InsertDocument(ctx context.Context, collection string, doc Document) (Document, error) {
	path, err := c.collectionsPath(collection)
	if err != nil {
		return nil, err
	}
	if collection == "" {
		return nil, errors.New("gosdk: collection name is required")
	}
	var out Document
	err = c.requestJSON(ctx, http.MethodPost, path+"/documents", nil, doc, &out)
	return out, err
}

// UpdateDocument replaces fields of the identified document.
func (c *Client) UpdateDocument(ctx context.Context, collection, id string, doc Document) (Document, error) {
	path, err := c.collectionsPath(collection)
	if err != nil {
		return nil, err
	}
	if collection == "" || id == "" {
		return nil, errors.New("gosdk: collection and document ID are required")
	}
	var out Document
	err = c.requestJSON(ctx, http.MethodPut, path+"/documents/"+url.PathEscape(id), nil, doc, &out)
	return out, err
}

// DeleteDocument removes a document; the success response is HTTP 204.
func (c *Client) DeleteDocument(ctx context.Context, collection, id string) error {
	path, err := c.collectionsPath(collection)
	if err != nil {
		return err
	}
	if collection == "" || id == "" {
		return errors.New("gosdk: collection and document ID are required")
	}
	return c.requestJSON(ctx, http.MethodDelete, path+"/documents/"+url.PathEscape(id), nil, nil, nil)
}
