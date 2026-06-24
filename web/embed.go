// Package web embeds the SSE demo page so cmd/notify-local can serve it
// directly — no separate "open this file" step.
package web

import "embed"

//go:embed index.html
var FS embed.FS
