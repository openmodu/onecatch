package updatehelper

import (
	"archive/zip"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestExtractNativeBundle(t *testing.T) {
	for _, name := range []string{"OneCatch.app/Contents/MacOS/onecatch", "../escaped", "/escaped"} {
		t.Run(name, func(t *testing.T) {
			archive := filepath.Join(t.TempDir(), "update.zip")
			file, err := os.Create(archive)
			if err != nil {
				t.Fatal(err)
			}
			writer := zip.NewWriter(file)
			header := &zip.FileHeader{Name: name}
			header.SetMode(0o755)
			entry, err := writer.CreateHeader(header)
			if err != nil {
				t.Fatal(err)
			}
			_, _ = entry.Write([]byte("application"))
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			file.Close()
			destination := t.TempDir()
			err = extractBundle(archive, destination)
			if !filepath.IsLocal(name) {
				if err == nil {
					t.Fatal("unsafe archive accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			info, err := os.Stat(filepath.Join(destination, name))
			if err != nil || info.Mode().Perm()&0o111 == 0 {
				t.Fatalf("executable mode lost: %v", err)
			}
		})
	}
}

func TestBundleReplacementAndRollback(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture")
	}
	for _, ready := range []bool{true, false} {
		t.Run(fmt.Sprint(ready), func(t *testing.T) {
			target := filepath.Join(t.TempDir(), "OneCatch.app")
			executable := filepath.Join(target, "Contents", "MacOS", "onecatch")
			if err := os.MkdirAll(filepath.Dir(executable), 0o755); err != nil {
				t.Fatal(err)
			}
			old := "#!/bin/sh\nexit 0\n"
			if err := os.WriteFile(executable, []byte(old), 0o755); err != nil {
				t.Fatal(err)
			}
			payload := filepath.Join(t.TempDir(), "OneCatch.app")
			staged := filepath.Join(payload, "Contents", "MacOS", "onecatch")
			if err := os.MkdirAll(filepath.Dir(staged), 0o755); err != nil {
				t.Fatal(err)
			}
			updated := "#!/bin/sh\n# updated\nexit 0\n"
			if ready {
				updated = "#!/bin/sh\nprintf ready > \"$ONECATCH_UPDATE_READY_FILE\"\n"
			}
			if err := os.WriteFile(staged, []byte(updated), 0o755); err != nil {
				t.Fatal(err)
			}
			timeout := 300 * time.Millisecond
			if ready {
				timeout = 5 * time.Second
			}
			err := replaceBundleWithTimeout(payload, target, filepath.Join(t.TempDir(), "ready"), timeout)
			if ready && err != nil || !ready && err == nil {
				t.Fatalf("ready=%v: %v", ready, err)
			}
			data, err := os.ReadFile(executable)
			if err != nil {
				t.Fatal(err)
			}
			want := old
			if ready {
				want = updated
			}
			if string(data) != want {
				t.Fatalf("installed content: %q", data)
			}
		})
	}
}
