// 文件说明：internal/sbcli object / site 命令实现（原 commands_resources.go 拆出）
//
// 单 key 的 object delete 在 commands.go 的命令表里用 idResult 适配，不在本文件重复。
package sbcli

import (
	"bytes"
	"context"
	"encoding/base64"
	"flag"
	"io"
	"os"
	"strings"
	"time"

	gosdk "github.com/linkxzhou/SimpleBase/packages/go-sdk"
)

func cmdObjectList(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	f, _, err := mustFlags(args, "prefix")
	if err != nil {
		return nil, err
	}
	return c.ListObjects(ctx, *f["prefix"], false)
}

func cmdObjectUpload(ctx context.Context, c *gosdk.Client, cfg loadedConfig, args []string, stdin io.Reader) (any, error) {
	f, _, err := mustFlags(args, "key", "file", "content-base64")
	if err != nil {
		return nil, err
	}
	key, err := need(f, "key")
	if err != nil {
		return nil, err
	}
	path, err := need(f, "file")
	if err != nil {
		return nil, err
	}
	// planv6.0 D3：助手模式宿主机文件不可读；内容经 --content-base64
	// 内联或 --file -（stdin）传入。人类操作者路径行为不变。
	if cfg.Agent && path != "-" && strings.TrimSpace(*f["content-base64"]) == "" {
		return nil, usagef("file_not_allowed: agent mode requires --file - (stdin) or --content-base64")
	}
	if b64 := strings.TrimSpace(*f["content-base64"]); b64 != "" {
		b, err := base64.StdEncoding.DecodeString(b64)
		if err != nil {
			return nil, usagef("--content-base64: invalid base64")
		}
		return c.UploadObject(ctx, key, bytes.NewReader(b), path, "")
	}
	var r io.Reader
	if path == "-" {
		r = io.LimitReader(stdin, 50<<20)
	} else {
		fh, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer fh.Close()
		r = fh
	}
	return c.UploadObject(ctx, key, r, path, "")
}

func cmdObjectPresign(ctx context.Context, c *gosdk.Client, cfg loadedConfig, args []string, _ io.Reader) (any, error) {
	f, _, err := mustFlags(args, "key")
	if err != nil {
		return nil, err
	}
	key, err := need(f, "key")
	if err != nil {
		return nil, err
	}
	res, err := c.PresignObject(ctx, key)
	if err != nil {
		return nil, err
	}
	out := map[string]any{"key": key, "expires_at": time.Now().UTC().Add(15 * time.Minute).Format(time.RFC3339)}
	if !cfg.Agent {
		out["url"] = res.URL
	}
	return out, nil
}

// sitePublishPrefix 是助手唯一允许公开的对象前缀（home-starter-prompts-plan §2.5）：
// 禁止公开对象根或其它目录，避免一次确认就把整个项目文件暴露到外网。
const sitePublishPrefix = "web/"

func cmdSiteStatus(ctx context.Context, c *gosdk.Client, _ loadedConfig, _ []string, _ io.Reader) (any, error) {
	return c.GetSite(ctx)
}

func cmdSitePublish(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	f, _, err := mustFlags(args, "prefix")
	if err != nil {
		return nil, err
	}
	prefix := strings.TrimSpace(*f["prefix"])
	if prefix == "" || prefix == "/" {
		return nil, usagef("site_root_publish_forbidden: --prefix %s is required; the object root cannot be published", sitePublishPrefix)
	}
	if prefix != sitePublishPrefix {
		return nil, usagef("site_prefix_not_allowed: only --prefix %s can be published", sitePublishPrefix)
	}
	return c.SetPublicFolder(ctx, prefix, true)
}

// cmdSiteEnable 公开约定站点前缀 web/；站点没有实例级总开关。
func cmdSiteEnable(ctx context.Context, c *gosdk.Client, _ loadedConfig, _ []string, _ io.Reader) (any, error) {
	return c.SetPublicFolder(ctx, sitePublishPrefix, true)
}

// cmdSiteDisable 取消约定站点前缀 web/ 的公开。
func cmdSiteDisable(ctx context.Context, c *gosdk.Client, _ loadedConfig, _ []string, _ io.Reader) (any, error) {
	return c.SetPublicFolder(ctx, sitePublishPrefix, false)
}

// cmdObjectPublic 设置单文件 / 文件夹的外部访问；文件夹 key 必须以 "/" 结尾。
func cmdObjectPublic(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	set := flag.NewFlagSet("simplebase", flag.ContinueOnError)
	set.SetOutput(io.Discard)
	key := set.String("key", "", "")
	kind := set.String("kind", "file", "")
	on := set.Bool("on", false, "")
	off := set.Bool("off", false, "")
	if err := set.Parse(args); err != nil {
		return nil, usagef("%s", err.Error())
	}
	if strings.TrimSpace(*key) == "" {
		return nil, usagef("--key is required")
	}
	if *on == *off {
		return nil, usagef("use exactly one of --on or --off")
	}
	if *kind == "folder" && !strings.HasSuffix(*key, "/") {
		return nil, usagef("folder key must end with /")
	}
	return c.SetPublic(ctx, *key, *kind, *on)
}
