package lsp

import (
	"runtime"
	"strings"
)

// uriFromPathFor / pathFromURIFor are pure — goos is a parameter and no
// host-dependent filepath calls are made — so every platform's URI shape
// is testable from any platform (the unitFor pattern).
//
// LSP requires file:///C:/... (three slashes) for drive-letter paths;
// "file://" + path is only correct when the path already starts with "/",
// which is true on unix and false for C:\... on Windows.

func uriFromPath(abs string) string { return uriFromPathFor(runtime.GOOS, abs) }

func uriFromPathFor(goos, abs string) string {
	p := abs
	if goos == "windows" {
		p = strings.ReplaceAll(p, `\`, "/")
		if !strings.HasPrefix(p, "/") {
			p = "/" + p
		}
	}
	return "file://" + p
}

func pathFromURI(uri string) string { return pathFromURIFor(runtime.GOOS, uri) }

func pathFromURIFor(goos, uri string) string {
	p, ok := strings.CutPrefix(uri, "file://")
	if !ok {
		return uri
	}
	if goos == "windows" {
		// file:///C:/x → /C:/x → C:\x (servers may lowercase the drive).
		// URIs without a drive prefix have no native Windows rendering —
		// pass them through untouched rather than backslashifying them.
		if len(p) >= 3 && p[0] == '/' && p[2] == ':' {
			p = strings.ReplaceAll(p[1:], "/", `\`)
		}
	}
	return p
}
