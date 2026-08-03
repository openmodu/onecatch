package assets

import "embed"

// Frontend contains the production Vite bundle generated from the root
// frontend directory.
//
//go:embed all:frontend/dist
var Frontend embed.FS

//go:embed appicon.png
var AppIcon []byte
