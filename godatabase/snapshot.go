package godatabase

import "time"

type SnapshotInfo struct {
	Engine      string    `json:"engine"`
	FormatVer   int       `json:"format_version"`
	SizeBytes   int64     `json:"size_bytes"`
	SHA256      string    `json:"sha256"`
	CompletedAt time.Time `json:"completed_at"`
}
