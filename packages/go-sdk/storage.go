package gosdk

import (
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"path"
	"strings"
)

// ObjectMeta describes a stored object. LastModified may be absent.
type ObjectMeta struct {
	Key          string `json:"key"`
	Size         int64  `json:"size"`
	LastModified string `json:"lastModified,omitempty"`
}

// PresignResult is a temporary download URL (currently valid for 15 minutes).
type PresignResult struct {
	URL string `json:"url"`
}

// DeleteObjectResult is the storage delete acknowledgement.
type DeleteObjectResult struct {
	OK bool `json:"ok"`
}

// ListObjects returns up to 1000 project objects; refresh bypasses the metadata cache.
func (c *Client) ListObjects(ctx context.Context, prefix string, refresh bool) ([]ObjectMeta, error) {
	q := url.Values{}
	if prefix != "" {
		q.Set("prefix", prefix)
	}
	if refresh {
		q.Set("refresh", "1")
	}
	var out []ObjectMeta
	err := c.requestJSON(ctx, http.MethodGet, c.projectPath("/s3/objects"), q, nil, &out)
	return out, err
}

// UploadObject streams content as multipart. The caller retains ownership of body.
// A context cancellation stops the upload; contentType becomes the file-part Content-Type.
func (c *Client) UploadObject(ctx context.Context, key string, body io.Reader, filename, contentType string) (*ObjectMeta, error) {
	if err := validateObjectKey(key); err != nil {
		return nil, err
	}
	if body == nil {
		return nil, errors.New("gosdk: upload body is required")
	}
	if filename == "" {
		filename = path.Base(key)
	}
	pr, pw := io.Pipe()
	writer := multipart.NewWriter(pw)
	go func() {
		var err error
		defer func() { pw.CloseWithError(err) }()
		if err = writer.WriteField("key", key); err != nil {
			return
		}
		header := make(textproto.MIMEHeader)
		header.Set("Content-Disposition", `form-data; name="file"; filename="`+escapeMultipartFilename(filename)+`"`)
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		header.Set("Content-Type", contentType)
		var part io.Writer
		part, err = writer.CreatePart(header)
		if err != nil {
			return
		}
		_, err = io.Copy(part, body)
		if err != nil {
			return
		}
		err = writer.Close()
	}()
	var out ObjectMeta
	err := c.request(ctx, http.MethodPost, c.projectPath("/s3/objects"), nil, pr, writer.FormDataContentType(), &out)
	pr.CloseWithError(err)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func escapeMultipartFilename(s string) string {
	return strings.NewReplacer("\\", "_", "\"", "_", "\r", "_", "\n", "_").Replace(s)
}

func validateObjectKey(key string) error {
	if key == "" || len(key) > 1024 || strings.HasPrefix(key, "/") || strings.ContainsAny(key, "\\\x00") {
		return errors.New("gosdk: invalid object key")
	}
	for _, segment := range strings.Split(key, "/") {
		if segment == "." || segment == ".." {
			return errors.New("gosdk: invalid object key")
		}
	}
	return nil
}

// DeleteObject deletes an object by its original project-scoped key.
func (c *Client) DeleteObject(ctx context.Context, key string) (*DeleteObjectResult, error) {
	if err := validateObjectKey(key); err != nil {
		return nil, err
	}
	var out DeleteObjectResult
	err := c.requestJSON(ctx, http.MethodDelete, c.projectPath("/s3/objects"), url.Values{"key": {key}}, nil, &out)
	return &out, err
}

// PresignObject creates a temporary download link.
func (c *Client) PresignObject(ctx context.Context, key string) (*PresignResult, error) {
	if err := validateObjectKey(key); err != nil {
		return nil, err
	}
	var out PresignResult
	err := c.requestJSON(ctx, http.MethodGet, c.projectPath("/s3/presign"), url.Values{"key": {key}}, nil, &out)
	return &out, err
}
