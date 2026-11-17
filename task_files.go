package mwanachamataskmanager

import (
	"fmt"
	"strings"
	"unicode"
)

const (
	maxFiles      = 100
	maxFileLength = 300
)

func checkFiles(files FileList) (FileList, error) {
	if len(files) > maxFiles {
		return nil, fmt.Errorf("%w: %d entries, at most %d", ErrInvalidFiles, len(files), maxFiles)
	}
	seen := map[string]bool{}
	out := FileList{}
	for _, raw := range files {
		entry := strings.TrimSpace(raw)
		if err := checkFileEntry(entry); err != nil {
			return nil, err
		}
		if !seen[entry] {
			seen[entry] = true
			out = append(out, entry)
		}
	}
	return out, nil
}

func checkFileEntry(entry string) error {
	switch {
	case entry == "":
		return fmt.Errorf("%w: an entry is blank", ErrInvalidFiles)
	case len([]rune(entry)) > maxFileLength:
		return fmt.Errorf("%w: %q is longer than %d characters", ErrInvalidFiles, entry, maxFileLength)
	case strings.IndexFunc(entry, unicode.IsControl) >= 0:
		return fmt.Errorf("%w: %q holds a control character", ErrInvalidFiles, entry)
	}
	path := pathOfFileEntry(entry)
	switch {
	case path == "":
		return fmt.Errorf("%w: %q names no path", ErrInvalidFiles, entry)
	case strings.HasPrefix(path, "/"):
		return fmt.Errorf("%w: %q starts with a slash", ErrInvalidFiles, entry)
	case hasParentSegment(path):
		return fmt.Errorf("%w: %q holds a .. segment", ErrInvalidFiles, entry)
	}
	return nil
}

func pathOfFileEntry(entry string) string {
	repo, path, prefixed := strings.Cut(entry, ":")
	if prefixed && repo != "" && !strings.Contains(repo, "/") {
		return path
	}
	return entry
}

func hasParentSegment(path string) bool {
	for _, segment := range strings.Split(path, "/") {
		if segment == ".." {
			return true
		}
	}
	return false
}
