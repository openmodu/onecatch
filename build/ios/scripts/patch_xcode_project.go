package main

import (
	"bytes"
	"fmt"
	"os"
)

var (
	generatedBuild   = []byte(`go build -buildmode=c-archive -overlay build/ios/xcode/overlay.json -o \"bin/OneCatch.a\"`)
	fixedBuild       = []byte(`/bin/sh build/ios/scripts/build_xcode_archive.sh`)
	generatedSigning = []byte(`CODE_SIGNING_ALLOWED = NO;`)
	fixedSigning     = []byte(`CODE_SIGN_STYLE = Automatic;`)
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
	changed := false
	if !bytes.Contains(content, fixedBuild) {
		if !bytes.Contains(content, generatedBuild) {
			fmt.Fprintln(os.Stderr, "generated Xcode build phase has an unknown Go command")
			os.Exit(1)
		}
		content = bytes.Replace(content, generatedBuild, fixedBuild, 1)
		changed = true
	}
	if bytes.Contains(content, generatedSigning) {
		content = bytes.ReplaceAll(content, generatedSigning, fixedSigning)
		changed = true
	} else if !bytes.Contains(content, fixedSigning) {
		fmt.Fprintln(os.Stderr, "generated Xcode project has unknown signing settings")
		os.Exit(1)
	}
	if !changed {
		return
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
