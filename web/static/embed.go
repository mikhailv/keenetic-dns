package static

import "embed"

//go:embed app.js index.html
var FS embed.FS
