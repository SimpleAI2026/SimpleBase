package ducklake

import (
	"fmt"
	"strings"

	"github.com/linkxzhou/SimpleBase/internal/objectstore"
)

// RemoteStorage 描述 Phase 2 的 S3 持久层。Enabled=false 时走本地 DATA_PATH（DevMode / Phase 1）。
type RemoteStorage struct {
	Enabled        bool
	Endpoint       string
	Region         string
	Bucket         string
	RootPrefix     string
	Environment    string
	AccessKey      string
	SecretKey      string
	ForcePathStyle bool
	UseSSL         bool
}

func (r RemoteStorage) keyBuilder() objectstore.KeyBuilder {
	env := r.Environment
	if env == "" {
		env = "dev"
	}
	prefix := r.RootPrefix
	if prefix == "" {
		prefix = "simplebase"
	}
	return objectstore.KeyBuilder{RootPrefix: prefix, Environment: env}
}

func (r RemoteStorage) validate() error {
	if !r.Enabled {
		return nil
	}
	if r.Bucket == "" {
		return fmt.Errorf("ducklake: remote bucket is required")
	}
	if r.Region == "" {
		return fmt.Errorf("ducklake: remote region is required")
	}
	return nil
}

func (r RemoteStorage) s3EndpointHost() string {
	ep := strings.TrimSpace(r.Endpoint)
	ep = strings.TrimPrefix(ep, "https://")
	ep = strings.TrimPrefix(ep, "http://")
	ep = strings.TrimSuffix(ep, "/")
	return ep
}

func (r RemoteStorage) endpointUsesSSL() bool {
	ep := strings.TrimSpace(r.Endpoint)
	if strings.HasPrefix(ep, "http://") {
		return false
	}
	if strings.HasPrefix(ep, "https://") {
		return true
	}
	if ep == "" {
		return true
	}
	return r.UseSSL
}
