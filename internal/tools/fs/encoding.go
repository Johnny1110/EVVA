package fs

import (
	"os"
	"path/filepath"
	"strings"
	"unicode/utf16"
)

// fileEncoding tracks how the file was read so we can write it back in
// the same form. UTF-16 LE files are auto-detected via BOM (Windows /
// Notepad default); everything else is assumed UTF-8.
type fileEncoding int

const (
	encUTF8 fileEncoding = iota
	encUTF16LE
)

// lineEndings records what terminator the file used so callers that
// want to roundtrip the original shape (Edit) can restore it on write.
// Bare-CR (classic Mac) is rare enough that we just collapse to LF.
type lineEndings int

const (
	endLF lineEndings = iota
	endCRLF
)

// fileInMemory is the normalized, LF-only string view callers operate
// on. enc + lf are what we use to restore on write.
type fileInMemory struct {
	content string
	enc     fileEncoding
	lf      lineEndings
}

// readFileWithEncoding reads bytes from disk, detects encoding (UTF-16
// LE BOM, UTF-8 BOM), normalizes CRLF→LF in memory, and returns the
// decoded text plus the original encoding / line-ending kind so the
// caller can roundtrip on write.
//
// Three text-tool callers share this:
//   - Read: presents normalized text to the model.
//   - Edit: edits the normalized form, restores CRLF + encoding.
//   - Write: needs original encoding for overwrite re-encoding and
//     prior-content for the diff.
func readFileWithEncoding(path string) (fileInMemory, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fileInMemory{}, err
	}

	enc := encUTF8
	var decoded string
	switch {
	case len(raw) >= 2 && raw[0] == 0xff && raw[1] == 0xfe:
		enc = encUTF16LE
		decoded = decodeUTF16LE(raw[2:])
	case len(raw) >= 3 && raw[0] == 0xef && raw[1] == 0xbb && raw[2] == 0xbf:
		// UTF-8 BOM — strip so the model's old_string doesn't need to
		// include it. We don't roundtrip the BOM on write because it
		// causes more compatibility issues than it solves.
		decoded = string(raw[3:])
	default:
		decoded = string(raw)
	}

	lf := endLF
	if strings.Contains(decoded, "\r\n") {
		lf = endCRLF
		decoded = strings.ReplaceAll(decoded, "\r\n", "\n")
	}

	return fileInMemory{content: decoded, enc: enc, lf: lf}, nil
}

// writeFileWithEncoding writes content back to disk re-applying the
// file's original encoding. CRLF restoration is opt-in via restoreCRLF
// — Edit passes true (in-place mutation should preserve the file's
// shape); Write passes false (full replacement honors the model's
// explicit line endings in content).
func writeFileWithEncoding(path, content string, enc fileEncoding, restoreCRLF bool) error {
	if restoreCRLF {
		content = strings.ReplaceAll(content, "\n", "\r\n")
	}
	switch enc {
	case encUTF16LE:
		return os.WriteFile(path, encodeUTF16LE(content), 0o644)
	default:
		return os.WriteFile(path, []byte(content), 0o644)
	}
}

func decodeUTF16LE(b []byte) string {
	n := len(b) / 2
	u16 := make([]uint16, 0, n)
	for i := 0; i < n; i++ {
		u16 = append(u16, uint16(b[2*i])|uint16(b[2*i+1])<<8)
	}
	return string(utf16.Decode(u16))
}

func encodeUTF16LE(s string) []byte {
	u16 := utf16.Encode([]rune(s))
	out := make([]byte, 0, 2+2*len(u16))
	out = append(out, 0xff, 0xfe) // BOM
	for _, c := range u16 {
		out = append(out, byte(c), byte(c>>8))
	}
	return out
}

// binaryExtensions is ported from ref/src/constants/files.ts
// BINARY_EXTENSIONS. Read rejects these to keep raw binary content
// out of the conversation context. PDF and image extensions are
// excluded at the call site since the Read tool renders them natively.
var binaryExtensions = map[string]struct{}{
	// Images
	".png": {}, ".jpg": {}, ".jpeg": {}, ".gif": {}, ".bmp": {}, ".ico": {},
	".webp": {}, ".tiff": {}, ".tif": {},
	// Video
	".mp4": {}, ".mov": {}, ".avi": {}, ".mkv": {}, ".webm": {}, ".wmv": {},
	".flv": {}, ".m4v": {}, ".mpeg": {}, ".mpg": {},
	// Audio
	".mp3": {}, ".wav": {}, ".ogg": {}, ".flac": {}, ".aac": {}, ".m4a": {},
	".wma": {}, ".aiff": {}, ".opus": {},
	// Archives
	".zip": {}, ".tar": {}, ".gz": {}, ".bz2": {}, ".7z": {}, ".rar": {},
	".xz": {}, ".z": {}, ".tgz": {}, ".iso": {},
	// Executables / native binaries
	".exe": {}, ".dll": {}, ".so": {}, ".dylib": {}, ".bin": {}, ".o": {},
	".a": {}, ".obj": {}, ".lib": {}, ".app": {}, ".msi": {}, ".deb": {}, ".rpm": {},
	// Documents (PDF is here; Read excludes it at the call site)
	".pdf": {}, ".doc": {}, ".docx": {}, ".xls": {}, ".xlsx": {}, ".ppt": {},
	".pptx": {}, ".odt": {}, ".ods": {}, ".odp": {},
	// Fonts
	".ttf": {}, ".otf": {}, ".woff": {}, ".woff2": {}, ".eot": {},
	// Bytecode / VM artifacts
	".pyc": {}, ".pyo": {}, ".class": {}, ".jar": {}, ".war": {}, ".ear": {},
	".node": {}, ".wasm": {}, ".rlib": {},
	// Database files
	".sqlite": {}, ".sqlite3": {}, ".db": {}, ".mdb": {}, ".idx": {},
	// Design / 3D
	".psd": {}, ".ai": {}, ".eps": {}, ".sketch": {}, ".fig": {}, ".xd": {},
	".blend": {}, ".3ds": {}, ".max": {},
	// Flash
	".swf": {}, ".fla": {},
	// Lock / profiling data
	".lockb": {}, ".dat": {}, ".data": {},
}

// hasBinaryExtension reports whether path's extension is in the
// known-binary blocklist. Case-insensitive on the extension.
func hasBinaryExtension(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	_, ok := binaryExtensions[ext]
	return ok
}

// blockedDevicePaths is the set of device files that would hang the
// process: infinite output or blocking input. /dev/null is safe and
// intentionally omitted. Ported from ref FileReadTool.BLOCKED_DEVICE_PATHS.
var blockedDevicePaths = map[string]struct{}{
	// Infinite output — never reach EOF
	"/dev/zero":    {},
	"/dev/random":  {},
	"/dev/urandom": {},
	"/dev/full":    {},
	// Blocks waiting for input
	"/dev/stdin":   {},
	"/dev/tty":     {},
	"/dev/console": {},
	// Nonsensical to read
	"/dev/stdout": {},
	"/dev/stderr": {},
	// fd aliases for stdio
	"/dev/fd/0": {},
	"/dev/fd/1": {},
	"/dev/fd/2": {},
}

// isBlockedDevicePath reports whether path is a device that would
// hang or produce infinite output. Linux /proc/<pid>/fd/0-2 are also
// caught as stdio aliases.
func isBlockedDevicePath(path string) bool {
	if _, ok := blockedDevicePaths[path]; ok {
		return true
	}
	if strings.HasPrefix(path, "/proc/") {
		if strings.HasSuffix(path, "/fd/0") || strings.HasSuffix(path, "/fd/1") || strings.HasSuffix(path, "/fd/2") {
			return true
		}
	}
	return false
}
