package ducklake

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/objectstore"
)

// Manifest 是 catalog 指针序列的一项（multi-instance-consistency-plan §4.4）。
// manifest/{seq:020d}.json 不可变（PutIfAbsent 写入）；seq 单调递增，
// 第二个 writer 对同一 seq 的写入必然得到 ErrPreconditionFailed —— 这就是
// split-brain 检测，只依赖 create-if-absent，无需 CAS。
//
// ducklake-duckdb-catalog-plan §3.1 新增字段：
//   - CatalogEngine：写该快照时的 catalog 引擎；读取方与配置不一致 → 拒绝；
//   - Size/SHA256：快照对象的长度与内容摘要，下载后 rename 前校验。
type Manifest struct {
	Seq           int64  `json:"seq"`
	SnapshotID    int64  `json:"snapshot_id"`
	SnapshotKey   string `json:"snapshot_key"`
	WriterEpoch   int64  `json:"writer_epoch"`
	CatalogEngine string `json:"catalog_engine,omitempty"`
	Size          int64  `json:"size,omitempty"`
	SHA256        string `json:"sha256,omitempty"`
	UpdatedAt     string `json:"updated_at"`
}

// manifestSeqFromKey 从 manifest/{seq}.json key 解析 seq；非数字段返回 0。
func manifestSeqFromKey(key string) int64 {
	seg := key
	if i := strings.LastIndexByte(key, '/'); i >= 0 {
		seg = key[i+1:]
	}
	seg = strings.TrimSuffix(seg, ".json")
	n, err := strconv.ParseInt(seg, 10, 64)
	if err != nil {
		return 0
	}
	return n
}

// ErrManifestChainBroken 表示 manifest 连续链不变量被破坏
// （ducklake-storage-latency-optimization-v2.0-plan §3.1）：
// manifest seq 从 1 连续递增且永不删除；锚点缺失、或 seq 1 缺失但更高
// seq 存在，都说明链被外部删除/篡改。发现断链必须 fail closed ——
// 拒绝打开/拒写，不得退回旧镜像。
var ErrManifestChainBroken = errors.New("ducklake: manifest chain broken")

// IsManifestChainBroken 判定错误是否为 manifest 断链。
func IsManifestChainBroken(err error) bool {
	return errors.Is(err, ErrManifestChainBroken)
}

// maxManifestSeqProbe 是指数探测的 seq 上界（2^32，实际不可达）；
// 超过即视为异常，fail closed。
const maxManifestSeqProbe = int64(1) << 32

// ReadLatestManifest 返回当前最大 seq 的 manifest。
// 未找到返回 (nil, nil)——新库尚无任何快照。
//
// 探测策略（依赖「seq 从 1 连续递增、不可删除」的不变量）：
//  1. fromSeq > 0：锚点校验——调用方声称已知的链上一点必须存在，
//     缺失即断链（ErrManifestChainBroken），不得悄悄回退；
//  2. fromSeq <= 0：探测 seq 1；缺失时用指数探测确认更高 seq 是否存在
//     （存在即断链 → fail closed；全部缺失才是新库/旧部署）；
//  3. 从锚点指数探测上界 + 二分定位最大 seq：O(log N) 请求，
//     与历史同步次数无关（替代旧的「最多前探 64 次」，后者在长历史时
//     会返回陈旧 seq 甚至被修剪后的空洞误导）。
func ReadLatestManifest(ctx context.Context, store objectstore.BlobStore, remote RemoteStorage, tenantID, databaseID string, fromSeq int64) (*Manifest, error) {
	kb := remote.keyBuilder()
	anchor := fromSeq
	var latest *Manifest
	if anchor > 0 {
		m, err := getManifestAt(ctx, store, kb, tenantID, databaseID, anchor)
		if err != nil {
			return nil, err
		}
		if m == nil {
			return nil, fmt.Errorf("%w: tenant=%s db=%s anchor seq %d missing",
				ErrManifestChainBroken, tenantID, databaseID, anchor)
		}
		latest = m
	} else {
		anchor = 1
		m, err := getManifestAt(ctx, store, kb, tenantID, databaseID, 1)
		if err != nil {
			return nil, err
		}
		if m == nil {
			// seq 1 缺失：新库，或历史部署删除过 seq 1（断链）。
			// 指数探测更高 seq；任一存在即断链，fail closed。
			broken, err := manifestExistsBeyond(ctx, store, kb, tenantID, databaseID, 1)
			if err != nil {
				return nil, err
			}
			if broken {
				return nil, fmt.Errorf("%w: tenant=%s db=%s seq 1 missing but later seq exists",
					ErrManifestChainBroken, tenantID, databaseID)
			}
			return nil, nil // 确认无任何 manifest：新库或兼容期旧部署（catalog.sqlite 形态）。
		}
		latest = m
	}

	// 指数探测缺失上界 hi（lo 必然存在）。
	lo := anchor
	step := int64(1)
	hi := int64(-1)
	for {
		probe := lo + step
		if probe > maxManifestSeqProbe {
			probe = maxManifestSeqProbe
		}
		m, err := getManifestAt(ctx, store, kb, tenantID, databaseID, probe)
		if err != nil {
			return nil, err
		}
		if m == nil {
			hi = probe
			break
		}
		latest = m
		lo = probe
		if probe >= maxManifestSeqProbe {
			return nil, fmt.Errorf("%w: tenant=%s db=%s seq exceeds probe bound 2^32",
				ErrManifestChainBroken, tenantID, databaseID)
		}
		step *= 2
	}
	// 二分定位 (lo, hi) 内最大存在的 seq。
	for lo+1 < hi {
		mid := lo + (hi-lo)/2
		m, err := getManifestAt(ctx, store, kb, tenantID, databaseID, mid)
		if err != nil {
			return nil, err
		}
		if m == nil {
			hi = mid
		} else {
			latest = m
			lo = mid
		}
	}
	return latest, nil
}

// manifestExistsBeyond 指数探测 seq 之后是否存在任何 manifest（断链判定用）。
func manifestExistsBeyond(ctx context.Context, store objectstore.BlobStore, kb objectstore.KeyBuilder, tenantID, databaseID string, seq int64) (bool, error) {
	for probe := seq + 1; probe <= maxManifestSeqProbe; {
		m, err := getManifestAt(ctx, store, kb, tenantID, databaseID, probe)
		if err != nil {
			return false, err
		}
		if m != nil {
			return true, nil
		}
		if probe > maxManifestSeqProbe/2 {
			break
		}
		probe *= 2
	}
	return false, nil
}

func getManifestAt(ctx context.Context, store objectstore.BlobStore, kb objectstore.KeyBuilder, tenantID, databaseID string, seq int64) (*Manifest, error) {
	key, err := kb.DuckLakeManifestKey(tenantID, databaseID, seq)
	if err != nil {
		return nil, err
	}
	data, _, err := store.GetBytes(ctx, key)
	if err != nil {
		if errors.Is(err, objectstore.ErrNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("ducklake: get manifest %d: %w", seq, err)
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("ducklake: parse manifest %d: %w", seq, err)
	}
	if m.Seq != seq {
		return nil, fmt.Errorf("%w: manifest key seq %d differs from payload seq %d", ErrManifestChainBroken, seq, m.Seq)
	}
	// 旧格式（无 catalog_engine）视为不兼容历史数据（ducklake-duckdb-catalog-plan §3.1）。
	if m.CatalogEngine == "" {
		return nil, fmt.Errorf("%w: manifest seq %d has no catalog_engine field (legacy data written before engine tagging)", ErrCatalogEngineUnknown, seq)
	}
	return &m, nil
}

// WriteManifest 用 PutIfAbsent 写入 manifest/{seq}.json。
// 返回 ErrPreconditionFailed 时表示存在第二个 writer（split-brain 确证）。
func WriteManifest(ctx context.Context, store objectstore.BlobStore, remote RemoteStorage, tenantID, databaseID string, m Manifest) error {
	kb := remote.keyBuilder()
	key, err := kb.DuckLakeManifestKey(tenantID, databaseID, m.Seq)
	if err != nil {
		return err
	}
	body, err := json.Marshal(m)
	if err != nil {
		return err
	}
	if _, err := store.PutIfAbsent(ctx, key, body, "application/json"); err != nil {
		if errors.Is(err, objectstore.ErrPreconditionFailed) {
			return fmt.Errorf("ducklake: manifest seq %d conflict (split-brain): %w", m.Seq, objectstore.ErrPreconditionFailed)
		}
		return fmt.Errorf("ducklake: put manifest %d: %w", m.Seq, err)
	}
	return nil
}

// NewManifest 构造带时间戳与引擎/校验信息的 manifest。
func NewManifest(seq, snapshotID int64, snapshotKey, catalogEngine string, writerEpoch int64, size int64, sha256 string) Manifest {
	return Manifest{
		Seq:           seq,
		SnapshotID:    snapshotID,
		SnapshotKey:   snapshotKey,
		WriterEpoch:   writerEpoch,
		CatalogEngine: NormalizeCatalogEngine(catalogEngine),
		Size:          size,
		SHA256:        sha256,
		UpdatedAt:     time.Now().UTC().Format(time.RFC3339Nano),
	}
}
