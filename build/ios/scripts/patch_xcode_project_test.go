package main

import (
	"strings"
	"testing"
)

func TestPatchProjectAddsAppIconResources(t *testing.T) {
	project := `
/* Begin PBXBuildFile section */
/* End PBXBuildFile section */
/* Begin PBXFileReference section */
/* End PBXFileReference section */
				C0DEBEEF0000000000000003 /* Info.plist */,
			buildPhases = (
				C0DEBEEF0000000000000056 /* Frameworks */,
			);
			buildRules = (
/* Begin PBXShellScriptBuildPhase section */
shellScript = "go build -buildmode=c-archive -overlay build/ios/xcode/overlay.json -o \"bin/OneCatch.a\"";
				CODE_SIGNING_ALLOWED = NO;
				INFOPLIST_FILE = main/Info.plist;
`
	patched, changed, err := patchProject([]byte(project))
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("project was not patched")
	}
	text := string(patched)
	for _, expected := range []string{
		"Assets.xcassets in Resources", "PBXResourcesBuildPhase", "ASSETCATALOG_COMPILER_APPICON_NAME = AppIcon;",
		"/bin/sh build/ios/scripts/build_xcode_archive.sh", "CODE_SIGN_STYLE = Automatic;",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("patched project is missing %q", expected)
		}
	}

	again, changed, err := patchProject(patched)
	if err != nil {
		t.Fatal(err)
	}
	if changed || string(again) != text {
		t.Fatal("patch must be idempotent")
	}
}
