package remotefs

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestCompleteDirectories(t *testing.T) {
	client, closeServer := startLocalSFTPServer(t)
	defer closeServer()
	root := t.TempDir()
	for _, name := range []string{"project a", "project-b", "项目", ".hidden"} {
		if err := os.Mkdir(filepath.Join(root, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "project-file"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "project a"), filepath.Join(root, "project-link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "missing"), filepath.Join(root, "project-broken")); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		prefix string
		names  []string
	}{
		{"project", []string{"project a", "project-b", "project-link"}},
		{"项目", []string{"项目"}},
		{".", []string{".hidden"}},
		{"absent", []string{}},
	} {
		t.Run(tc.prefix, func(t *testing.T) {
			got, err := completeDirectories(client, root+"/"+tc.prefix)
			if err != nil {
				t.Fatal(err)
			}
			want := []string{}
			for _, name := range tc.names {
				want = append(want, filepath.Join(root, name)+"/")
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("got %q, want %q", got, want)
			}
		})
	}
	home, err := client.RealPath(".")
	if err != nil {
		t.Fatal(err)
	}
	want, err := completeDirectories(client, home+"/")
	if err != nil {
		t.Fatal(err)
	}
	for _, prefix := range []string{"", "~", "~/"} {
		got, err := completeDirectories(client, prefix)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("home %q: got %q, err %v", prefix, got, err)
		}
	}
	if _, err := completeDirectories(client, root+"/missing/"); err == nil {
		t.Fatal("missing directory should fail")
	}
	if _, err := completeDirectories(client, "bad\x00path"); err == nil {
		t.Fatal("invalid path should fail")
	}
}
