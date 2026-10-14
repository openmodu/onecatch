package remotefs

import (
	"context"
	"fmt"
	"os"
	"path"
	"sort"
	"strings"

	"github.com/pkg/sftp"
)

// CompleteDirectories lists matching directories without running a remote shell.
// Results are absolute paths so they can be saved as workspace roots directly.
func CompleteDirectories(ctx context.Context, config SFTPConfig, prefix string) ([]string, error) {
	config.Root = "/"
	backend, err := NewSFTPBackend(ctx, config)
	if err != nil {
		return nil, err
	}
	defer backend.Close()
	return completeDirectories(backend.client, prefix)
}

func completeDirectories(client *sftp.Client, prefix string) ([]string, error) {
	if strings.IndexByte(prefix, 0) >= 0 {
		return nil, fmt.Errorf("invalid path")
	}
	if !path.IsAbs(prefix) {
		home, err := client.RealPath(".")
		if err != nil {
			return nil, err
		}
		if prefix == "~" {
			prefix = ""
		} else {
			prefix = strings.TrimPrefix(prefix, "~/")
		}
		prefix = strings.TrimRight(home, "/") + "/" + prefix
	}
	parent, base := path.Split(prefix)
	entries, err := client.ReadDir(parent)
	if err != nil {
		return nil, err
	}
	results := []string{}
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), base) {
			continue
		}
		candidate := path.Join(parent, entry.Name())
		if !entry.IsDir() {
			if entry.Mode()&os.ModeSymlink == 0 {
				continue
			}
			info, err := client.Stat(candidate)
			if err != nil || !info.IsDir() {
				continue
			}
		}
		results = append(results, candidate+"/")
	}
	sort.Strings(results)
	return results, nil
}
