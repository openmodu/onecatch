package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
)

var (
	generatedBuild   = []byte(`go build -buildmode=c-archive -overlay build/ios/xcode/overlay.json -o \"bin/OneCatch.a\"`)
	fixedBuild       = []byte(`/bin/sh build/ios/scripts/build_xcode_archive.sh`)
	generatedSigning = []byte(`CODE_SIGNING_ALLOWED = NO;`)
	fixedSigning     = []byte(`CODE_SIGN_STYLE = Automatic;`)
	assetsBuildFile  = []byte("\t\tC0DEBEEF0000000000000006 /* Assets.xcassets in Resources */ = {isa = PBXBuildFile; fileRef = C0DEBEEF0000000000000005 /* Assets.xcassets */; };\n")
	assetsFileRef    = []byte("\t\tC0DEBEEF0000000000000005 /* Assets.xcassets */ = {isa = PBXFileReference; lastKnownFileType = folder.assetcatalog; path = Assets.xcassets; sourceTree = \"<group>\"; };\n")
	assetsGroupChild = []byte("\t\t\t\tC0DEBEEF0000000000000005 /* Assets.xcassets */,\n")
	resourcesPhase   = []byte(`/* Begin PBXResourcesBuildPhase section */
		C0DEBEEF0000000000000057 /* Resources */ = {
			isa = PBXResourcesBuildPhase;
			buildActionMask = 2147483647;
			files = (
				C0DEBEEF0000000000000006 /* Assets.xcassets in Resources */,
			);
			runOnlyForDeploymentPostprocessing = 0;
		};
/* End PBXResourcesBuildPhase section */

`)
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
	content, changed, err := patchProject(content)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
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

func patchProject(content []byte) ([]byte, bool, error) {
	changed := false
	if !bytes.Contains(content, fixedBuild) {
		if !bytes.Contains(content, generatedBuild) {
			return nil, false, errors.New("generated Xcode build phase has an unknown Go command")
		}
		content = bytes.Replace(content, generatedBuild, fixedBuild, 1)
		changed = true
	}
	if bytes.Contains(content, generatedSigning) {
		content = bytes.ReplaceAll(content, generatedSigning, fixedSigning)
		changed = true
	} else if !bytes.Contains(content, fixedSigning) {
		return nil, false, errors.New("generated Xcode project has unknown signing settings")
	}

	// Wails' generated iOS target has no Resources phase. The catalog exists on
	// disk, but without these project entries Xcode never runs actool and the
	// signed device bundle reaches SpringBoard with no application icon.
	var err error
	content, changed, err = insertBefore(content, []byte("/* End PBXBuildFile section */"), assetsBuildFile, []byte("Assets.xcassets in Resources"), changed)
	if err != nil {
		return nil, false, err
	}
	content, changed, err = insertBefore(content, []byte("/* End PBXFileReference section */"), assetsFileRef, []byte("/* Assets.xcassets */ = {isa = PBXFileReference"), changed)
	if err != nil {
		return nil, false, err
	}
	content, changed, err = insertBefore(content, []byte("\t\t\t\tC0DEBEEF0000000000000003 /* Info.plist */,"), assetsGroupChild, []byte("C0DEBEEF0000000000000005 /* Assets.xcassets */,"), changed)
	if err != nil {
		return nil, false, err
	}
	content, changed, err = insertBefore(content, []byte("\t\t\t);\n\t\t\tbuildRules"), []byte("\t\t\t\tC0DEBEEF0000000000000057 /* Resources */,\n"), []byte("C0DEBEEF0000000000000057 /* Resources */,"), changed)
	if err != nil {
		return nil, false, err
	}
	content, changed, err = insertBefore(content, []byte("/* Begin PBXShellScriptBuildPhase section */"), resourcesPhase, []byte("/* Begin PBXResourcesBuildPhase section */"), changed)
	if err != nil {
		return nil, false, err
	}
	if !bytes.Contains(content, []byte("ASSETCATALOG_COMPILER_APPICON_NAME = AppIcon;")) {
		marker := []byte("\t\t\t\tINFOPLIST_FILE = main/Info.plist;")
		if !bytes.Contains(content, marker) {
			return nil, false, errors.New("generated Xcode project has no Info.plist build setting")
		}
		content = bytes.ReplaceAll(content, marker, append([]byte("\t\t\t\tASSETCATALOG_COMPILER_APPICON_NAME = AppIcon;\n"), marker...))
		changed = true
	}
	return content, changed, nil
}

func insertBefore(content, marker, addition, present []byte, changed bool) ([]byte, bool, error) {
	if bytes.Contains(content, present) {
		return content, changed, nil
	}
	if !bytes.Contains(content, marker) {
		return nil, false, fmt.Errorf("generated Xcode project is missing marker %q", marker)
	}
	return bytes.Replace(content, marker, append(append([]byte{}, addition...), marker...), 1), true, nil
}
