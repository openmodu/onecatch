package main

import (
	"bytes"
	"fmt"
	"os"
)

var (
	generatedBuild = []byte(`go build -buildmode=c-archive -overlay build/ios/xcode/overlay.json -o \"bin/OneCatch.a\"`)
	fixedBuild     = []byte(`/bin/sh build/ios/scripts/build_xcode_archive.sh`)
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: patch_xcode_project <project.pbxproj>")
		os.Exit(2)
	}
	path := os.Args[1]
	content, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if bytes.Contains(content, fixedBuild) {
		return
	}
	if !bytes.Contains(content, generatedBuild) {
		fmt.Fprintln(os.Stderr, "generated Xcode build phase has an unknown Go command")
		os.Exit(1)
	}
	content = bytes.Replace(content, generatedBuild, fixedBuild, 1)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
