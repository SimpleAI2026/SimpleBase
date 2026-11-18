// 文件说明：internal/sbcli backup 命令实现（原 commands_resources.go 拆出）
package sbcli

import (
	"context"
	"io"

	gosdk "github.com/linkxzhou/SimpleBase/packages/go-sdk"
)

func cmdBackupList(ctx context.Context, c *gosdk.Client, _ loadedConfig, _ []string, _ io.Reader) (any, error) {
	items, err := c.ListBackups(ctx)
	return map[string]any{"items": items}, err
}

func cmdBackupCreate(ctx context.Context, c *gosdk.Client, _ loadedConfig, _ []string, _ io.Reader) (any, error) {
	return c.CreateBackup(ctx)
}

func cmdBackupRestore(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	return backupIDCommand(ctx, args, c.RestoreBackup)
}

func cmdBackupDelete(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	return backupIDCommand(ctx, args, c.DeleteBackup)
}

func backupIDCommand(ctx context.Context, args []string, action func(context.Context, string) error) (any, error) {
	f, _, err := mustFlags(args, "id")
	if err != nil {
		return nil, err
	}
	id, err := need(f, "id")
	if err != nil {
		return nil, err
	}
	if err := action(ctx, id); err != nil {
		return nil, err
	}
	return map[string]any{"ok": true}, nil
}
