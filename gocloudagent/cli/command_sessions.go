// 文件说明：command_sessions.go — `sessions` 子命令：list / show / rm

package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func cmdSessions(env []string, args []string, stdout, stderr *os.File) int {
	sub := "list"
	if len(args) > 0 {
		sub = args[0]
	}
	switch sub {
	case "list", "ls":
		sessions, err := ListSessions(env)
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return exitConfig
		}
		if len(sessions) == 0 {
			fmt.Fprintln(stdout, "no sessions yet")
			return exitOK
		}
		for _, s := range sessions {
			fmt.Fprintf(stdout, "%s  %s  %s\n", s.ID, s.CreatedAt.Local().Format(time.DateTime), s.Cwd)
		}
		return exitOK
	case "show":
		if len(args) < 2 {
			fmt.Fprintln(stderr, "usage: gocloudagent sessions show <session-id>")
			return exitUsage
		}
		path, err := FindSession(env, args[1])
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return exitConfig
		}
		s, err := OpenSession(path)
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return exitConfig
		}
		for _, e := range s.Lines {
			switch e.Type {
			case EntrySession:
				fmt.Fprintf(stdout, "session %s  cwd=%s  endpoint=%s  model=%s\n", s.ID, e.Cwd, e.Endpoint, e.Model)
			case EntryCompaction:
				fmt.Fprintf(stdout, "[compaction] %s\n", firstLine(e.Summary, 120))
			default:
				fmt.Fprintf(stdout, "%s: %s\n", e.Role, firstLine(e.Content, 200))
			}
		}
		return exitOK
	case "rm", "remove", "delete":
		if len(args) < 2 {
			fmt.Fprintln(stderr, "usage: gocloudagent sessions rm <session-id>")
			return exitUsage
		}
		path, err := FindSession(env, args[1])
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return exitConfig
		}
		if err := RemoveSession(path); err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return exitConfig
		}
		fmt.Fprintln(stdout, "removed", filepath.Base(path))
		return exitOK
	default:
		fmt.Fprintf(stderr, "unknown sessions subcommand %q\n", sub)
		return exitUsage
	}
}
