package keeldesktop

import (
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dyike/keel/gio/gpu/headless"
	"github.com/dyike/keel/gio/layout"
	"github.com/dyike/keel/gio/op"
	"github.com/dyike/keel/gio/unit"
	"github.com/dyike/keel/ui/el"
)

// Opt-in native GPU measurement. Run each dependency revision in a fresh
// process; compare footprints separately from allocation-rate benchmarks.
func TestConversationMemoryProfile(t *testing.T) {
	dir := os.Getenv("ONECATCH_MEMORY_PROFILE")
	if dir == "" {
		t.Skip("set ONECATCH_MEMORY_PROFILE to write native GPU diagnostics")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	a := conversationFixture(t)
	a.transcript.items["answer"].text = strings.Repeat(benchAnswer, 8)
	a.syncMessages()
	root := el.Root(a)
	size := image.Pt(5016, 2820)
	w, err := headless.NewWindow(size.X, size.Y)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Release()
	var ops op.Ops
	for range 12 {
		ops.Reset()
		root.Layout(layout.Context{Ops: &ops, Now: time.Now(), Constraints: layout.Exact(size), Metric: unit.Metric{PxPerDp: 2, PxPerSp: 2}})
		if err := w.Frame(&ops); err != nil {
			t.Fatal(err)
		}
	}
	runtime.GC()
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	t.Logf("heap_alloc=%d heap_inuse=%d heap_sys=%d", stats.HeapAlloc, stats.HeapInuse, stats.HeapSys)
	if runtime.GOOS == "darwin" {
		out, err := exec.Command("vmmap", "-summary", strconv.Itoa(os.Getpid())).CombinedOutput()
		if err != nil {
			t.Fatalf("vmmap: %v: %s", err, out)
		}
		if err := os.WriteFile(filepath.Join(dir, "vmmap.txt"), out, 0600); err != nil {
			t.Fatal(err)
		}
	}
	f, err := os.Create(filepath.Join(dir, "heap.pprof"))
	if err != nil {
		t.Fatal(err)
	}
	if err := pprof.WriteHeapProfile(f); err != nil {
		t.Fatal(err)
	}
	f.Close()
	// Readback allocates additional buffers, so measure before taking the image.
	pixels := image.NewRGBA(image.Rectangle{Max: size})
	if err := w.Screenshot(pixels); err != nil {
		t.Fatal(err)
	}
	f, err = os.Create(filepath.Join(dir, "frame.png"))
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, pixels); err != nil {
		t.Fatal(err)
	}
	f.Close()
	runtime.KeepAlive(root)
}
