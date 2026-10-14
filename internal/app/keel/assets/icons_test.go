package assets

import (
	"strings"
	"testing"

	"github.com/dyike/keel/ui/kit"
)

func TestEmbeddedIconsParse(t *testing.T) {
	for _, dir := range []string{"icons", "runtimeicons"} {
		entries, err := iconAssets.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if !strings.HasSuffix(entry.Name(), ".svg") {
				continue
			}
			name := dir + "/" + entry.Name()
			data, _ := iconAssets.ReadFile(name)
			if _, err := kit.SVGIcon(data); err != nil {
				t.Errorf("%s: %v", name, err)
			}
		}
	}
}
