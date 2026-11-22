package image

import (
	"encoding/json"
	"errors"
	"fmt"
)

// OCI 与 Docker 的 media type。
const (
	// ManifestListType 是多架构索引（OCI index 或 Docker manifest list）。
	ManifestListType = "application/vnd.oci.image.index.v1+json"
	// DockerListType 是 Docker 的 manifest list。
	DockerListType = "application/vnd.docker.distribution.manifest.list.v2+json"
	// ManifestType 是单平台 OCI manifest。
	ManifestType = "application/vnd.oci.image.manifest.v1+json"
	// DockerManifestType 是单平台 Docker manifest（schema 2）。
	DockerManifestType = "application/vnd.docker.distribution.manifest.v2+json"

	// LayerType 与 DockerLayerType 是层的 media type（gzip / 未压缩）。
	LayerType       = "application/vnd.oci.image.layer.v1.tar+gzip"
	LayerTypeZstd   = "application/vnd.oci.image.layer.v1.tar+zstd"
	DockerLayerType = "application/vnd.docker.image.rootfs.diff.tar.gzip"
	// LayerTypePlain 是未压缩层；导入的镜像常见。
	LayerTypePlain = "application/vnd.oci.image.layer.v1.tar"
	// ConfigType 与 DockerConfigType 是镜像 config 的 media type。
	ConfigType       = "application/vnd.oci.image.config.v1+json"
	DockerConfigType = "application/vnd.docker.container.image.v1+json"
)

// ErrUnsupportedPlatform 表示镜像没有目标平台的构建。
var ErrUnsupportedPlatform = errors.New("image: no build for this platform")

// Descriptor 是 manifest 里的一个对象引用。
type Descriptor struct {
	MediaType   string            `json:"mediaType"`
	Digest      string            `json:"digest"`
	Size        int64             `json:"size"`
	Platform    *PlatformRef      `json:"platform,omitempty"`
	URLs        []string          `json:"urls,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

// PlatformRef 是索引里标注的平台。
type PlatformRef struct {
	Architecture string `json:"architecture"`
	OS           string `json:"os"`
	Variant      string `json:"variant,omitempty"`
	OSVersion    string `json:"os.version,omitempty"`
}

// Manifest 同时容纳索引与单平台 manifest：按 MediaType 区分。
type Manifest struct {
	SchemaVersion int    `json:"schemaVersion"`
	MediaType     string `json:"mediaType"`
	// 索引
	Manifests []Descriptor `json:"manifests,omitempty"`
	// 单平台 manifest
	Config Descriptor   `json:"config,omitempty"`
	Layers []Descriptor `json:"layers,omitempty"`
	// 摘要与大小由调用方从响应头填入（不在 JSON 里）。
	Digest string `json:"-"`
	Size   int64  `json:"-"`
	// Raw 是原始字节，写进 store 时原样保存。
	Raw []byte `json:"-"`
}

// IsIndex 报告这是否是多架构索引。
func (m Manifest) IsIndex() bool {
	return m.MediaType == ManifestListType || m.MediaType == DockerListType ||
		(m.MediaType == "" && len(m.Manifests) > 0)
}

// DecodeManifest 解析 manifest 字节。mediaType 缺失时按内容推断。
func DecodeManifest(body []byte) (Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(body, &m); err != nil {
		return m, fmt.Errorf("image: parse manifest: %w", err)
	}
	if m.SchemaVersion != 2 {
		// schema 1 早已废弃，遇到就明说，而不是解析出半个结果。
		return m, fmt.Errorf("image: unsupported manifest schema %d", m.SchemaVersion)
	}
	if m.MediaType == "" {
		if len(m.Manifests) > 0 {
			m.MediaType = ManifestListType
		} else {
			m.MediaType = ManifestType
		}
	}
	m.Raw = body
	return m, nil
}

// PickPlatform 从索引里挑一份目标平台的 manifest。
//
// 只认 linux/<arch>：其它平台的镜像在本运行时里跑不了，挑错会让用户拿到
// 一个启动就报 exec format error 的根。
func (m Manifest) PickPlatform(arch, osName string) (Descriptor, bool) {
	for _, d := range m.Manifests {
		if d.Platform == nil {
			continue
		}
		if d.Platform.Architecture == arch && d.Platform.OS == osName {
			return d, true
		}
	}
	return Descriptor{}, false
}

// Config 是镜像 config 里本运行时用到的字段。
//
// 只取启动所需的：环境、入口、工作目录、用户、停止信号。
// 其余字段（history、rootfs 等）留在 store 里的原始 JSON 中。
type Config struct {
	// Digest 是 config 对象的摘要，也是「镜像 id」。
	Digest       string              `json:"-"`
	Env          []string            `json:"Env"`
	Entrypoint   []string            `json:"Entrypoint"`
	Cmd          []string            `json:"Cmd"`
	WorkingDir   string              `json:"WorkingDir"`
	User         string              `json:"User"`
	StopSignal   string              `json:"StopSignal"`
	Labels       map[string]string   `json:"Labels"`
	ExposedPorts map[string]struct{} `json:"ExposedPorts"`
	Volumes      map[string]struct{} `json:"Volumes"`
}

// imageConfigEnvelope 是 config JSON 的外层：真正的字段在 config 下。
type imageConfigEnvelope struct {
	Architecture string `json:"architecture"`
	OS           string `json:"os"`
	Config       Config `json:"config"`
}

// DecodeConfig 解析镜像 config JSON。
func DecodeConfig(body []byte) (Config, error) {
	var env imageConfigEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return Config{}, fmt.Errorf("image: parse config: %w", err)
	}
	return env.Config, nil
}
