package web

import (
	"embed"
	"io/fs"
)

//go:embed build/*
var embeddedFS embed.FS

var BuildFS fs.FS

func init() {
	var err error
	if BuildFS, err = fs.Sub(embeddedFS, "build"); err != nil {
		panic(err)
	}
}
