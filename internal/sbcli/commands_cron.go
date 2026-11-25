// 文件说明：internal/sbcli cron 命令实现（原 commands_resources.go 拆出）
package sbcli

import (
	"context"
	"io"
	"strconv"
	"strings"

	gosdk "github.com/linkxzhou/SimpleBase/packages/go-sdk"
)

func cronInput(f map[string]*string) (gosdk.CronJobInput, error) {
	in := gosdk.CronJobInput{
		Name: *f["name"], Description: *f["description"], ScheduleKind: *f["schedule-kind"],
		CronExpr: *f["cron"], RunAt: *f["run-at"], FuncFile: *f["func-file"], FuncExport: *f["func-export"],
		InputJSON: *f["input"],
	}
	if s := strings.TrimSpace(*f["interval-seconds"]); s != "" {
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return in, usagef("--interval-seconds must be an integer")
		}
		in.IntervalSeconds = n
	}
	if s := strings.TrimSpace(*f["enabled"]); s != "" {
		b, err := strconv.ParseBool(s)
		if err != nil {
			return in, usagef("--enabled must be true or false")
		}
		in.Enabled = &b
	}
	return in, nil
}

func cronFlags(args []string) (map[string]*string, error) {
	f, _, err := mustFlags(args, "id", "name", "description", "schedule-kind", "cron", "interval-seconds", "run-at", "func-file", "func-export", "input", "enabled")
	return f, err
}

func cmdCronList(ctx context.Context, c *gosdk.Client, _ loadedConfig, _ []string, _ io.Reader) (any, error) {
	raw, err := c.ListCronJobs(ctx)
	return decodeRaw(raw), err
}

func cmdCronCreate(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	f, err := cronFlags(args)
	if err != nil {
		return nil, err
	}
	in, err := cronInput(f)
	if err != nil {
		return nil, err
	}
	raw, err := c.CreateCronJob(ctx, in)
	return decodeRaw(raw), err
}

func cmdCronUpdate(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	f, err := cronFlags(args)
	if err != nil {
		return nil, err
	}
	id, err := need(f, "id")
	if err != nil {
		return nil, err
	}
	in, err := cronInput(f)
	if err != nil {
		return nil, err
	}
	raw, err := c.UpdateCronJob(ctx, id, in)
	return decodeRaw(raw), err
}
