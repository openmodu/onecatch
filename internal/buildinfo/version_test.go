package buildinfo

import "testing"

func TestDevelopmentVersionFallback(t *testing.T) {
	if Version == "" {
		t.Fatal("Version must never be empty")
	}
}
