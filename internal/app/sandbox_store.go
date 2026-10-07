package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/sandbox"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

// sandboxStoreAdapter 在 app 装配层隔离 sandbox.Manager 与系统库具体实现。
type sandboxStoreAdapter struct{ s *systemdb.Store }

func toSandbox(r systemdb.SandboxRow) sandbox.Sandbox {
	out := sandbox.Sandbox{ID: r.ID, ProjectID: r.ProjectID, Name: r.Name, CloudName: r.CloudName, Source: r.Source,
		ThreadID: r.ThreadID, Image: r.Image, CPUs: r.CPUs, MemoryMiB: r.MemoryMiB, Network: r.Network,
		IdleTimeoutS: r.IdleTimeoutS, MaxDurationS: r.MaxDurationS, Status: r.Status, LastError: r.LastError,
		CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt}
	if !r.StartedAt.IsZero() {
		t := r.StartedAt
		out.StartedAt = &t
	}
	if !r.LastActiveAt.IsZero() {
		t := r.LastActiveAt
		out.LastActiveAt = &t
	}
	if !r.ExpiresAt.IsZero() {
		t := r.ExpiresAt
		out.ExpiresAt = &t
	}
	if !r.DeletedAt.IsZero() {
		t := r.DeletedAt
		out.DeletedAt = &t
	}
	return out
}
func toSandboxRow(r sandbox.Sandbox) systemdb.SandboxRow {
	out := systemdb.SandboxRow{ID: r.ID, ProjectID: r.ProjectID, Name: r.Name, CloudName: r.CloudName, Source: r.Source,
		ThreadID: r.ThreadID, Image: r.Image, CPUs: r.CPUs, MemoryMiB: r.MemoryMiB, Network: r.Network,
		IdleTimeoutS: r.IdleTimeoutS, MaxDurationS: r.MaxDurationS, Status: r.Status, LastError: r.LastError,
		CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt}
	if r.StartedAt != nil {
		out.StartedAt = *r.StartedAt
	}
	if r.LastActiveAt != nil {
		out.LastActiveAt = *r.LastActiveAt
	}
	if r.ExpiresAt != nil {
		out.ExpiresAt = *r.ExpiresAt
	}
	if r.DeletedAt != nil {
		out.DeletedAt = *r.DeletedAt
	}
	return out
}
func (a sandboxStoreAdapter) CreateSandbox(ctx context.Context, r sandbox.Sandbox) (sandbox.Sandbox, error) {
	row, err := a.s.CreateSandbox(ctx, toSandboxRow(r))
	return toSandbox(row), err
}
func (a sandboxStoreAdapter) GetSandbox(ctx context.Context, p, id string) (sandbox.Sandbox, error) {
	r, err := a.s.GetSandbox(ctx, p, id)
	return toSandbox(r), err
}
func (a sandboxStoreAdapter) GetSandboxByName(ctx context.Context, p, name string) (sandbox.Sandbox, error) {
	r, err := a.s.GetSandboxByName(ctx, p, name)
	return toSandbox(r), err
}
func (a sandboxStoreAdapter) GetSandboxByThread(ctx context.Context, p, thread string) (sandbox.Sandbox, error) {
	r, err := a.s.GetSandboxByThread(ctx, p, thread)
	return toSandbox(r), err
}
func (a sandboxStoreAdapter) ListSandboxes(ctx context.Context, p, status, source, cursor string, limit int) ([]sandbox.Sandbox, string, error) {
	rows, next, err := a.s.ListSandboxes(ctx, p, status, source, cursor, limit)
	if errors.Is(err, systemdb.ErrSandboxCursor) {
		return nil, "", fmt.Errorf("%w: cursor", sandbox.ErrInvalidSpec)
	}
	if err != nil {
		return nil, "", err
	}
	out := make([]sandbox.Sandbox, 0, len(rows))
	for _, r := range rows {
		out = append(out, toSandbox(r))
	}
	return out, next, nil
}
func (a sandboxStoreAdapter) CountSandboxes(ctx context.Context, p string) (int, error) {
	return a.s.CountSandboxes(ctx, p)
}
func (a sandboxStoreAdapter) UpdateSandbox(ctx context.Context, r sandbox.Sandbox) error {
	return a.s.UpdateSandbox(ctx, toSandboxRow(r))
}
func (a sandboxStoreAdapter) SoftDeleteSandbox(ctx context.Context, p, id, msg string) error {
	return a.s.SoftDeleteSandbox(ctx, p, id, msg)
}
func (a sandboxStoreAdapter) ListExpiredSandboxes(ctx context.Context, now time.Time, limit int) ([]sandbox.Sandbox, error) {
	rows, err := a.s.ListExpiredSandboxes(ctx, now, limit)
	if err != nil {
		return nil, err
	}
	out := make([]sandbox.Sandbox, 0, len(rows))
	for _, r := range rows {
		out = append(out, toSandbox(r))
	}
	return out, nil
}
func (a sandboxStoreAdapter) ListStaleRunSandboxes(ctx context.Context, before time.Time, limit int) ([]sandbox.Sandbox, error) {
	rows, err := a.s.ListStaleRunSandboxes(ctx, before, limit)
	if err != nil {
		return nil, err
	}
	out := make([]sandbox.Sandbox, 0, len(rows))
	for _, r := range rows {
		out = append(out, toSandbox(r))
	}
	return out, nil
}
func (a sandboxStoreAdapter) ListFailedSandboxRemovals(ctx context.Context, limit int) ([]sandbox.Sandbox, error) {
	rows, err := a.s.ListFailedSandboxRemovals(ctx, limit)
	if err != nil {
		return nil, err
	}
	out := make([]sandbox.Sandbox, 0, len(rows))
	for _, r := range rows {
		out = append(out, toSandbox(r))
	}
	return out, nil
}
func (a sandboxStoreAdapter) PurgeDeletedSandboxes(ctx context.Context, before time.Time) (int64, error) {
	return a.s.PurgeDeletedSandboxes(ctx, before)
}

var _ sandbox.Store = sandboxStoreAdapter{}
