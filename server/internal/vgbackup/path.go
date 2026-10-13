package vgbackup

import (
	"path"
	"path/filepath"
	"strings"
)

// VgIsAbsPath reports whether p is absolute under EITHER the host OS
// convention or slash convention. Storage target paths are AGENT-local:
// an agent on Linux reports "/var/backups" while the server may run on
// Windows (where filepath.IsAbs("/var/backups") is false) and vice versa.
// Accepting both spellings keeps validation portable; relative paths stay
// rejected everywhere (a relative path would resolve against an unknown
// working directory on whatever host interprets it).
func VgIsAbsPath(p string) bool {
	if filepath.IsAbs(p) {
		return true
	}
	return path.IsAbs(filepath.ToSlash(p))
}

// VgIsWithinBase reports whether child is within or equal to base, supporting
// both POSIX (/) and Windows (\) path formats across platforms.
func VgIsWithinBase(base, child string) bool {
	// Normalize both to slash convention first
	bSlash := path.Clean(filepath.ToSlash(base))
	cSlash := path.Clean(filepath.ToSlash(child))
	if cSlash == bSlash || strings.HasPrefix(cSlash, bSlash+"/") {
		return true
	}
	// A filesystem-root base contains every absolute path in the same
	// spelling (without this, HasPrefix(child, "//") is never true and a
	// root target would fail closed on every report).
	if bSlash == "/" && strings.HasPrefix(filepath.ToSlash(child), "/") {
		return true
	}

	// Also check native filepath convention
	bNative := filepath.Clean(base)
	cNative := filepath.Clean(child)
	if cNative == bNative || strings.HasPrefix(cNative, bNative+string(filepath.Separator)) {
		return true
	}
	// Native filesystem-root base ("/" on POSIX, `C:\` or `\` on Windows)
	// contains every absolute path on the same volume.
	if filepath.IsAbs(bNative) && filepath.Dir(bNative) == bNative &&
		filepath.IsAbs(cNative) && filepath.VolumeName(cNative) == filepath.VolumeName(bNative) {
		return true
	}

	return false
}
