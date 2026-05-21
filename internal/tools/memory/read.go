package memory

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
)

// maxReadBytes mirrors memdir.MaxFileBytes — duplicated here to keep the
// dependency arrow one-way (memdir.readMemFile is unexported on purpose).
const maxReadBytes = 64 * 1024

// readFileCapped reads `path` and returns (body, warning). Missing files
// return ("", "") so callers can scaffold cleanly. Oversize files are
// truncated to maxReadBytes with a warning so a bloated MEMORY.md doesn't
// silently lose data on the next write.
func readFileCapped(path string) (string, string) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", ""
		}
		return "", fmt.Sprintf("memory: cannot read %s: %v", path, err)
	}
	defer f.Close()
	buf, err := io.ReadAll(io.LimitReader(f, maxReadBytes+1))
	if err != nil {
		return "", fmt.Sprintf("memory: read %s: %v", path, err)
	}
	if len(buf) > maxReadBytes {
		return string(buf[:maxReadBytes]), fmt.Sprintf("memory: %s truncated to %d bytes", path, maxReadBytes)
	}
	return string(buf), ""
}
