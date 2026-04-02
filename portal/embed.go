package main

import (
	"embed"
	"io/fs"
)

//go:embed all:web/dist
var webDist embed.FS

func WebFS() fs.FS {
	sub, err := fs.Sub(webDist, "web/dist")
	if err != nil {
		return nil
	}
	return sub
}
