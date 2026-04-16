//go:build linux

package main

import (
	"log"
	"os"
	"strings"
)

// logBootEnv prints Linux desktop environment context at startup so bug
// reports include the info needed to diagnose Wayland/X11, compositor, and
// WebKit rendering issues without the reporter having to run extra commands.
func logBootEnv() {
	session := []string{"XDG_SESSION_TYPE", "XDG_CURRENT_DESKTOP", "WAYLAND_DISPLAY", "DISPLAY"}
	overrides := []string{"GDK_BACKEND", "GSK_RENDERER", "WEBKIT_DISABLE_DMABUF_RENDERER", "WEBKIT_DISABLE_COMPOSITING_MODE", "GTK_THEME"}
	sandbox := []string{"SNAP", "FLATPAK_ID", "container"}

	log.Printf("[desktop] session: %s", joinEnv(session, true))
	if s := joinEnv(overrides, false); s != "" {
		log.Printf("[desktop] render overrides: %s", s)
	}
	if s := joinEnv(sandbox, false); s != "" {
		log.Printf("[desktop] sandbox: %s", s)
	}
}

// joinEnv formats env vars as "KEY=value" pairs. When includeUnset is true,
// unset vars are rendered as "KEY=" so the reader can tell they were checked.
// When false, unset vars are omitted (noise reduction for overrides).
func joinEnv(keys []string, includeUnset bool) string {
	var parts []string
	for _, k := range keys {
		v := os.Getenv(k)
		if v == "" && !includeUnset {
			continue
		}
		parts = append(parts, k+"="+v)
	}
	return strings.Join(parts, " ")
}
