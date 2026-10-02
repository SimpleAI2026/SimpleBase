package goexample

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type FunctionInfo struct {
	Source        string `json:"source"`
	ActiveVersion int64  `json:"active_version"`
	LatestVersion int64  `json:"latest_version"`
}
type TestResult struct {
	OK         bool `json:"ok"`
	StatusCode int  `json:"status_code"`
}
type JobConfig struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	ScheduleKind string `json:"schedule_kind"`
	CronExpr     string `json:"cron_expr"`
	FuncFile     string `json:"func_file"`
	FuncExport   string `json:"func_export"`
	InputJSON    string `json:"input_json"`
	Enabled      bool   `json:"enabled"`
}

func (r *Runtime) Publish(ctx context.Context, caseDir, name, job, cron string, upload bool) error {
	source, err := os.ReadFile(filepath.Join(caseDir, "functions", name+".go"))
	if err != nil {
		return err
	}
	var info FunctionInfo
	endpoint := r.ProjectPath("/gofunctions/" + name)
	err = r.Request(ctx, http.MethodGet, endpoint, nil, &info)
	exists := err == nil
	if err != nil && !strings.Contains(err.Error(), "HTTP 404") {
		return err
	}
	if !exists || info.Source != string(source) || info.ActiveVersion == 0 {
		target := r.ProjectPath("/gofunctions")
		payload := map[string]any{"source": string(source), "activate": false}
		if !exists {
			payload["name"] = name
		} else {
			target += "/" + name + "/versions"
		}
		var created FunctionInfo
		if err := r.Request(ctx, http.MethodPost, target, payload, &created); err != nil {
			return err
		}
		version := created.LatestVersion
		if version <= 0 {
			return errors.New("invalid new function version")
		}
		for _, call := range []struct {
			Name string
			Body any
		}{{"Tick", map[string]any{}}, {"Compute", map[string]any{"rows": []any{}}}} {
			var test TestResult
			if err := r.Request(ctx, http.MethodPost, fmt.Sprintf("%s/versions/%d/test", endpoint, version), map[string]any{"function_name": call.Name, "body": call.Body}, &test); err != nil {
				return err
			}
			if !test.OK || test.StatusCode != 200 {
				return fmt.Errorf("%s test failed", call.Name)
			}
		}
		if err := r.Request(ctx, http.MethodPost, fmt.Sprintf("%s/versions/%d/activate", endpoint, version), nil, nil); err != nil {
			return err
		}
	}
	invokeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var confirmed map[string]any
	if err := r.Compute(invokeCtx, name, map[string]any{"rows": []any{}}, &confirmed); err != nil {
		return fmt.Errorf("activated function invoke failed: %w", err)
	}
	var jobs struct {
		Jobs []JobConfig `json:"jobs"`
	}
	if err := r.Request(ctx, http.MethodGet, r.ProjectPath("/cron-jobs"), nil, &jobs); err != nil {
		return err
	}
	config := JobConfig{Name: job, ScheduleKind: "cron", CronExpr: cron, FuncFile: name, FuncExport: "Tick", InputJSON: "{}", Enabled: true}
	target := r.ProjectPath("/cron-jobs")
	method := http.MethodPost
	for _, old := range jobs.Jobs {
		if old.Name == job {
			config.ID = old.ID
			method = http.MethodPatch
			target += "/" + old.ID
			if old.CronExpr == config.CronExpr && old.FuncFile == name && old.FuncExport == "Tick" && old.Enabled {
				method = ""
			}
			break
		}
	}
	if method != "" {
		if err := r.Request(ctx, method, target, config, nil); err != nil {
			return err
		}
	}
	if upload {
		return r.UploadDist(ctx, caseDir)
	}
	return nil
}
func (r *Runtime) UploadDist(ctx context.Context, caseDir string) error {
	dir := filepath.Join(caseDir, "dist")
	var paths []string
	if err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			return errors.New("symbolic links forbidden")
		}
		if entry.Type().IsRegular() {
			paths = append(paths, path)
		}
		return nil
	}); err != nil {
		return err
	}
	if len(paths) == 0 {
		return errors.New("empty frontend dist")
	}
	sort.Strings(paths)
	digest := sha256.New()
	for _, path := range paths {
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		_, _ = digest.Write(b)
	}
	prefix := filepath.Base(caseDir) + "/" + hex.EncodeToString(digest.Sum(nil))[:16] + "/"
	existing, err := r.Client.ListObjects(ctx, prefix, true)
	if err != nil {
		return err
	}
	if len(existing) > 0 {
		if len(existing) != len(paths) {
			return errors.New("partial upload already exists")
		}
		return nil
	}
	sort.SliceStable(paths, func(i, j int) bool {
		return filepath.Base(paths[i]) != "index.html" && filepath.Base(paths[j]) == "index.html"
	})
	for _, path := range paths {
		part, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		if strings.HasPrefix(part, "..") || filepath.IsAbs(part) {
			return errors.New("invalid asset path")
		}
		mime := map[string]string{".html": "text/html; charset=utf-8", ".js": "text/javascript", ".css": "text/css", ".json": "application/json", ".svg": "image/svg+xml"}[filepath.Ext(path)]
		if mime == "" {
			mime = "application/octet-stream"
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		_, uploadErr := r.Client.UploadObject(ctx, prefix+filepath.ToSlash(part), file, filepath.Base(path), mime)
		_ = file.Close()
		if uploadErr != nil {
			return uploadErr
		}
	}
	got, err := r.Client.ListObjects(ctx, prefix, true)
	if err != nil {
		return err
	}
	if len(got) != len(paths) {
		return errors.New("asset count mismatch")
	}
	return nil
}
