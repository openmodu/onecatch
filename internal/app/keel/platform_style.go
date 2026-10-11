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
		return "Segoe UI, Microsoft YaHei, Go, Segoe UI Emoji"
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
	for _, name := range []string{"segoeui.ttf", "segoeuib.ttf", "consola.ttf", "consolab.ttf", "msyh.ttc", "msyhbd.ttc", "seguiemj.ttf"} {
		path := filepath.Join(root, "Fonts", name)
		if info, err := os.Stat(path); err != nil || info.IsDir() {
			continue
		}
		paths = append(paths, path)
	}
	if len(paths) > 0 {
		if err := theme.LoadFontFilesWhere(func(font.Font) bool { return true }, paths...); err != nil {
			log.Printf("load Windows fonts: %v", err)
		}
	}
}

func platformWindowOptions(platform string, options window.Options) window.Options {
	options.Frameless = platform == "darwin"
	options.NativeTrafficLights = platform == "darwin"
	options.MenuDisplay = window.MenuDisplayAuto
	if platform != "darwin" {
		options.TrafficLightLayout = nil
	}
	return options
}
