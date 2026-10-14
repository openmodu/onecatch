package keeldesktop

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/dyike/keel/ui/core"
)

func (a *app) openConversationLink(raw string) {
	target, local, err := conversationLinkTarget(raw)
	if err != nil {
		a.fail(err)
		return
	}
	if !local {
		open := a.openURL
		if open == nil {
			open = core.OpenURL
		}
		a.fail(open(target))
		return
	}
	if !filepath.IsAbs(target) && !strings.HasPrefix(target, "~/") {
		w := a.workspaceForID(a.workspaceID)
		if w.RemoteFS != nil || a.task != nil && a.task.WorkerID != "" && a.task.WorkerID != "local" {
			a.fail(fmt.Errorf("无法在本机打开远端工作区的相对文件路径：%s", target))
			return
		}
		base := w.Path
		if a.task != nil && a.task.Worktree != nil {
			base = a.task.Worktree.Path
		}
		if base == "" {
			a.fail(fmt.Errorf("无法解析文件路径：当前没有工作区"))
			return
		}
		target = filepath.Join(base, target)
	}
	if strings.HasPrefix(target, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			a.fail(err)
			return
		}
		target = filepath.Join(home, target[2:])
	}
	target = filepath.Clean(target)
	// Prefer the exact filename; colons and # can legitimately occur in names.
	if _, err := os.Stat(target); err != nil {
		candidate := stripFileLocation(target)
		if candidate != target {
			if _, statErr := os.Stat(candidate); statErr == nil {
				target = candidate
				err = nil
			}
		}
		if err != nil {
			a.fail(fmt.Errorf("打开文件失败：%w", err))
			return
		}
	}
	open := a.openPath
	if open == nil {
		open = openLocalPath
	}
	a.fail(open(target))
}

func conversationLinkTarget(raw string) (string, bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false, fmt.Errorf("链接为空")
	}
	// Drive letters are filesystem syntax, not URL schemes.
	if len(raw) >= 3 && raw[1] == ':' && (raw[2] == '\\' || raw[2] == '/') {
		if runtime.GOOS != "windows" {
			return "", false, fmt.Errorf("无法在本机打开 Windows 文件路径：%s", raw)
		}
		return raw, true, nil
	}
	// Existing absolute paths can contain literal percent signs or #.
	if filepath.IsAbs(raw) {
		if _, err := os.Stat(raw); err == nil {
			return raw, true, nil
		}
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", false, fmt.Errorf("无效链接：%w", err)
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https", "mailto":
		return raw, false, nil
	case "file":
		if u.Host != "" && !strings.EqualFold(u.Host, "localhost") {
			return "", false, fmt.Errorf("不支持远端文件地址：%s", u.Host)
		}
		if u.Opaque != "" || u.Path == "" {
			return "", false, fmt.Errorf("无效文件链接：%s", raw)
		}
		p := u.Path
		if runtime.GOOS == "windows" && len(p) > 2 && p[0] == '/' && p[2] == ':' {
			p = p[1:]
		}
		return filepath.FromSlash(p), true, nil
	case "":
		if u.Host != "" {
			return "", false, fmt.Errorf("网页链接缺少 http 或 https 协议：%s", raw)
		}
		p, err := url.PathUnescape(raw)
		if err != nil {
			return "", false, err
		}
		return filepath.FromSlash(p), true, nil
	default:
		return "", false, fmt.Errorf("不支持的链接协议：%s", u.Scheme)
	}
}

func stripFileLocation(path string) string {
	if i := strings.LastIndex(path, "#L"); i >= 0 {
		if _, err := strconv.Atoi(path[i+2:]); err == nil {
			return path[:i]
		}
	}
	for range 2 {
		i := strings.LastIndexByte(path, ':')
		if i < 0 {
			break
		}
		n, err := strconv.Atoi(path[i+1:])
		if err != nil || n < 1 {
			break
		}
		path = path[:i]
	}
	return path
}

// Pass a filesystem argument directly, never through a shell or URL handler.
func openLocalPath(path string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", path)
	case "windows":
		cmd = exec.Command("explorer.exe", path)
	default:
		cmd = exec.Command("xdg-open", path)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("打开文件失败：%w", err)
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
