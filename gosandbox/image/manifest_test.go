package image

// manifest_test.go：manifest 与 config 解析、平台选择。
//
//   1. 索引与单平台 manifest 都能解析；
//   2. mediaType 缺失时按内容推断；
//   3. schema 1 被拒绝，而不是解析出半个结果；
//   4. 平台选择只认目标 arch/os；
//   5. config 里启动所需的字段解析正确。

import (
	"encoding/json"
	"errors"
	"testing"
)

// indexJSON 是一个两平台索引。
var indexJSON = `{
  "schemaVersion": 2,
  "mediaType": "application/vnd.oci.image.index.v1+json",
  "manifests": [
    {"mediaType":"application/vnd.oci.image.manifest.v1+json","digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","size":100,"platform":{"architecture":"arm64","os":"linux"}},
    {"mediaType":"application/vnd.oci.image.manifest.v1+json","digest":"sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","size":200,"platform":{"architecture":"amd64","os":"linux"}}
  ]
}`

func TestDecodeIndex(t *testing.T) {
	m, err := DecodeManifest([]byte(indexJSON))
	if err != nil {
		t.Fatal(err)
	}
	if !m.IsIndex() {
		t.Fatalf("must be an index, mediaType = %q", m.MediaType)
	}
	if len(m.Manifests) != 2 {
		t.Fatalf("manifests = %d", len(m.Manifests))
	}
	d, ok := m.PickPlatform("amd64", "linux")
	if !ok || d.Size != 200 {
		t.Fatalf("pick amd64 = %+v %v", d, ok)
	}
	if _, ok := m.PickPlatform("riscv64", "linux"); ok {
		t.Fatal("must not pick a platform that is absent")
	}
	if _, ok := m.PickPlatform("arm64", "windows"); ok {
		t.Fatal("must match the OS too")
	}
}

func TestDecodeManifestSingle(t *testing.T) {
	body := `{"schemaVersion":2,"mediaType":"application/vnd.oci.image.manifest.v1+json",
	  "config":{"mediaType":"application/vnd.oci.image.config.v1+json","digest":"sha256:` +
		hex64("c") + `","size":10},
	  "layers":[{"mediaType":"application/vnd.oci.image.layer.v1.tar+gzip","digest":"sha256:` +
		hex64("l") + `","size":20}]}`
	m, err := DecodeManifest([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if m.IsIndex() {
		t.Fatal("single manifest must not be an index")
	}
	if len(m.Layers) != 1 || m.Config.Digest == "" {
		t.Fatalf("manifest = %+v", m)
	}
}

// mediaType 缺失时按内容推断。
func TestDecodeManifestInfersType(t *testing.T) {
	index := `{"schemaVersion":2,"manifests":[{"digest":"sha256:` + hex64("a") + `","size":1}]}`
	m, err := DecodeManifest([]byte(index))
	if err != nil {
		t.Fatal(err)
	}
	if m.MediaType != ManifestListType {
		t.Fatalf("inferred %q", m.MediaType)
	}
	single := `{"schemaVersion":2,"layers":[{"digest":"sha256:` + hex64("a") + `","size":1}]}`
	m, err = DecodeManifest([]byte(single))
	if err != nil {
		t.Fatal(err)
	}
	if m.MediaType != ManifestType {
		t.Fatalf("inferred %q", m.MediaType)
	}
}

// schema 1 与损坏 JSON 都被拒绝。
func TestDecodeManifestRejects(t *testing.T) {
	if _, err := DecodeManifest([]byte(`{"schemaVersion":1}`)); err == nil {
		t.Fatal("schema 1 must be rejected")
	}
	if _, err := DecodeManifest([]byte(`{`)); err == nil {
		t.Fatal("broken JSON must be rejected")
	}
}

// config 解析：启动所需的字段。
func TestDecodeConfig(t *testing.T) {
	body := `{"architecture":"arm64","os":"linux","config":{
	  "Env":["PATH=/bin","HOME=/root"],
	  "Entrypoint":["/docker-entrypoint.sh"],
	  "Cmd":["nginx","-g","daemon off;"],
	  "WorkingDir":"/app",
	  "User":"1000:1000",
	  "StopSignal":"SIGQUIT",
	  "ExposedPorts":{"80/tcp":{}},
	  "Volumes":{"/data":{}}
	}}`
	cfg, err := DecodeConfig([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Env) != 2 || cfg.WorkingDir != "/app" || cfg.User != "1000:1000" {
		t.Fatalf("config = %+v", cfg)
	}
	if len(cfg.Entrypoint) != 1 || len(cfg.Cmd) != 3 {
		t.Fatalf("entrypoint/cmd = %+v / %+v", cfg.Entrypoint, cfg.Cmd)
	}
	if _, ok := cfg.ExposedPorts["80/tcp"]; !ok {
		t.Fatal("exposed ports must parse")
	}
	if _, ok := cfg.Volumes["/data"]; !ok {
		t.Fatal("volumes must parse")
	}
}

// 空 config 不报错，返回零值。
func TestDecodeConfigEmpty(t *testing.T) {
	cfg, err := DecodeConfig([]byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Env) != 0 || cfg.WorkingDir != "" {
		t.Fatalf("config = %+v", cfg)
	}
}

// Descriptor 的 JSON 往返（含 platform）。
func TestDescriptorRoundTrip(t *testing.T) {
	in := Descriptor{MediaType: ManifestType, Digest: "sha256:" + hex64("a"), Size: 5,
		Platform: &PlatformRef{Architecture: "arm64", OS: "linux", Variant: "v8"}}
	body, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out Descriptor
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	if out.Platform == nil || out.Platform.Variant != "v8" || out.Size != 5 {
		t.Fatalf("round trip = %+v", out)
	}
}

var _ = errors.Is
