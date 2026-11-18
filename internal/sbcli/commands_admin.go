// 文件说明：internal/sbcli log / user / project / apikey / settings / quota / audit 命令实现（原 commands_resources.go 拆出）
package sbcli

import (
	"context"
	"encoding/json"
	"io"
	"strconv"
	"strings"

	gosdk "github.com/linkxzhou/SimpleBase/packages/go-sdk"
)

func cmdLogSearch(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	f, _, err := mustFlags(args, "level", "q", "limit")
	if err != nil {
		return nil, err
	}
	limit, _ := strconv.Atoi(*f["limit"])
	raw, err := c.SearchLogs(ctx, *f["level"], *f["q"], limit)
	return decodeRaw(raw), err
}

func cmdUserList(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	f, _, err := mustFlags(args, "limit", "cursor")
	if err != nil {
		return nil, err
	}
	limit, _ := strconv.Atoi(*f["limit"])
	raw, err := c.ListUsers(ctx, limit, *f["cursor"])
	return decodeRaw(raw), err
}

func cmdUserGet(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	id, err := one(args, "id")
	if err != nil {
		return nil, err
	}
	raw, err := c.GetUser(ctx, id)
	return decodeRaw(raw), err
}

func cmdUserCreate(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, stdin io.Reader) (any, error) {
	if strings.Contains(strings.Join(args, " "), "--password") && !strings.Contains(strings.Join(args, " "), "--password-stdin") {
		return nil, usagef("passwords must be passed with --password-stdin")
	}
	f, _, err := mustFlags(args, "username", "role", "display-name", "email")
	if err != nil {
		return nil, err
	}
	user, err := need(f, "username")
	if err != nil {
		return nil, err
	}
	pw, err := io.ReadAll(io.LimitReader(stdin, 4096))
	if err != nil {
		return nil, err
	}
	raw, err := c.CreateUser(ctx, gosdk.UserInput{
		Username: user, Password: strings.TrimSpace(string(pw)), Role: *f["role"],
		DisplayName: *f["display-name"], Email: *f["email"],
	})
	return decodeRaw(raw), err
}

func cmdUserUpdate(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	f, _, err := mustFlags(args, "id", "role", "display-name", "email")
	if err != nil {
		return nil, err
	}
	id, err := need(f, "id")
	if err != nil {
		return nil, err
	}
	raw, err := c.UpdateUser(ctx, id, gosdk.UserInput{Role: *f["role"], DisplayName: *f["display-name"], Email: *f["email"]})
	return decodeRaw(raw), err
}

func cmdUserDisable(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	id, err := one(args, "id")
	if err != nil {
		return nil, err
	}
	st := "disabled"
	raw, err := c.UpdateUser(ctx, id, gosdk.UserInput{Status: &st})
	return decodeRaw(raw), err
}

func cmdUserDelete(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	id, err := one(args, "id")
	if err != nil {
		return nil, err
	}
	if err := c.DeleteUser(ctx, id); err != nil {
		return nil, err
	}
	return map[string]any{"ok": true}, nil
}

func cmdProjectList(ctx context.Context, c *gosdk.Client, _ loadedConfig, _ []string, _ io.Reader) (any, error) {
	raw, err := c.ListProjects(ctx)
	return decodeRaw(raw), err
}

func cmdProjectCreate(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	f, _, err := mustFlags(args, "name", "engine", "id")
	if err != nil {
		return nil, err
	}
	name, err := need(f, "name")
	if err != nil {
		return nil, err
	}
	raw, err := c.CreateProjectWithID(ctx, strings.TrimSpace(*f["id"]), name, strings.TrimSpace(*f["engine"]))
	return decodeRaw(raw), err
}

func cmdAPIKeyList(ctx context.Context, c *gosdk.Client, _ loadedConfig, _ []string, _ io.Reader) (any, error) {
	raw, err := c.ListAPIKeys(ctx)
	return decodeRaw(raw), err
}

func cmdAPIKeyCreate(ctx context.Context, c *gosdk.Client, cfg loadedConfig, args []string, _ io.Reader) (any, error) {
	f, _, err := mustFlags(args, "permissions")
	if err != nil {
		return nil, err
	}
	var perms []string
	if p := strings.TrimSpace(*f["permissions"]); p != "" {
		if err := json.Unmarshal([]byte(p), &perms); err != nil {
			perms = strings.Split(p, ",")
		}
	}
	key, err := c.CreateAPIKey(ctx, perms)
	if err != nil {
		return nil, err
	}
	out := map[string]any{"id": key.ID, "permissions": key.Permissions}
	if cfg.Agent {
		out["secret_withheld"] = true
	} else {
		out["secret"] = key.Secret
	}
	return out, nil
}

func cmdAPIKeyRevoke(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	id, err := one(args, "id")
	if err != nil {
		return nil, err
	}
	if err := c.RevokeAPIKey(ctx, id); err != nil {
		return nil, err
	}
	return map[string]any{"ok": true}, nil
}

func cmdSettingsGet(ctx context.Context, c *gosdk.Client, _ loadedConfig, _ []string, _ io.Reader) (any, error) {
	raw, err := c.GetSettings(ctx)
	return decodeRaw(raw), err
}

func cmdSettingsSet(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	rawJSON, err := one(args, "json")
	if err != nil {
		return nil, err
	}
	raw, err := c.PutSettings(ctx, json.RawMessage(rawJSON))
	return decodeRaw(raw), err
}

func cmdQuotaGet(ctx context.Context, c *gosdk.Client, _ loadedConfig, _ []string, _ io.Reader) (any, error) {
	raw, err := c.GetQuota(ctx)
	return decodeRaw(raw), err
}

func cmdAuditList(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	f, _, err := mustFlags(args, "limit")
	if err != nil {
		return nil, err
	}
	limit, _ := strconv.Atoi(*f["limit"])
	raw, err := c.ListAudit(ctx, limit)
	return decodeRaw(raw), err
}
