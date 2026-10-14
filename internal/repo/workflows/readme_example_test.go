package workflows

import (
	"os"
	"regexp"
	"testing"
)

// The README examples are the format's documentation; they must parse.
func TestReadmeWorkflowExamplesParse(t *testing.T) {
	for _, file := range []string{"../../../README.md", "../../../README.zh-CN.md"} {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		m := regexp.MustCompile("(?s)```yaml\n(.*?)```").FindSubmatch(data)
		if m == nil {
			t.Fatalf("%s has no workflow example", file)
		}
		if def, err := ParseWorkflowFile("example", m[1]); err != nil || len(def.Steps) != 5 {
			t.Fatalf("%s example: %v", file, err)
		}
	}
}
