package keeldesktop

import (
	"log"
	"os"
	"path/filepath"
	"runtime"

	"gioui.org/font"
	"github.com/dyike/keel/ui/theme"
	"github.com/dyike/keel/ui/window"
)

func platformUIFace(platform string) font.Typeface {
	if platform == "windows" {
		return "Segoe UI, Microsoft YaHei, Microsoft YaHei UI, SimSun, Go, Segoe UI Emoji"
	}
	return "SF Pro Text, SF Pro, .AppleSystemUIFont, Segoe UI, " + theme.Face
}

func loadPlatformFonts() {
	if runtime.GOOS != "windows" {
		return
	}
	root := os.Getenv("WINDIR")
	if root == "" {
		root = os.Getenv("SystemRoot")
	}
	if root == "" {
		return
	}
	// As in keel-rex, load installed Windows faces before the first frame.
	// Missing optional language packs retain Keel's system/Go fallback fonts.
	var paths []string
	for _, name := range []string{"segoeui.ttf", "segoeuib.ttf", "consola.ttf", "consolab.ttf", "msyh.ttc", "msyhbd.ttc", "simsun.ttc", "seguiemj.ttf"} {
		path := filepath.Join(root, "Fonts", name)
		if info, err := os.Stat(path); err != nil || info.IsDir() {
			continue
		}
		paths = append(paths, path)
	}
	if len(paths) > 0 {
		var files [][]byte
		for _, path := range paths {
			data, err := os.ReadFile(path)
			if err == nil {
				err = preferFontOutlines(data)
			}
			if err != nil {
				log.Printf("prepare Windows font %s: %v", path, err)
				continue
			}
			files = append(files, data)
		}
		if err := theme.LoadFontsWhere(func(font.Font) bool { return true }, files...); err != nil {
			log.Printf("load Windows fonts: %v", err)
		}
	}
}

func platformWindowOptions(platform string, options window.Options) window.Options {
	options.Frameless = platform == "darwin" || platform == "windows"
	options.NativeTrafficLights = platform == "darwin"
	options.MenuDisplay = window.MenuDisplayAuto
	if platform == "windows" {
		options.MenuDisplay = window.MenuDisplayHidden
	}
	if platform != "darwin" {
		options.TrafficLightLayout = nil
	}
	return options
}

func platformTerminalFace(platform string) font.Typeface {
	if platform == "windows" {
		return "Consolas, Microsoft YaHei, Microsoft YaHei UI, SimSun, Go Mono, Segoe UI Emoji"
	}
	return theme.MonoFace
}
