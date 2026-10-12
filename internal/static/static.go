package static

import (
	"embed"
	"io/fs"
)

// dist 由前端构建产出（vite build outDir 指向 internal/static/dist）
//
//go:embed all:dist
var distFS embed.FS

func FS() fs.FS {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic(err)
	}
	return sub
}
