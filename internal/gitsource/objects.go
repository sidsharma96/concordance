package gitsource

import (
	"bytes"
	"crypto/sha1" // Git SHA-1 object compatibility, not an authorization/security check.
	"crypto/sha256"
	"fmt"
	"hash"
	"path"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

type treeEntry struct {
	mode, id, name string
	size           int64
}

// parseListing checks the long, NUL-delimited ls-tree output before blob reads.
// Sorting provides stable path order and adjacent duplicate detection without
// assuming Git's directory traversal order is ordinary string order.
func parseListing(data []byte, maxEntries, idLength int) ([]treeEntry, error) {
	var entries []treeEntry
	for len(data) > 0 {
		if len(entries) >= maxEntries {
			return nil, ErrLimit
		}
		record, rest, terminated := bytes.Cut(data, []byte{0})
		if !terminated {
			return nil, ErrCorrupt
		}
		header, name, found := bytes.Cut(record, []byte{'\t'})
		if !found || len(header) > 128 || !cleanPath(string(name), false) {
			return nil, ErrCorrupt
		}
		fields := strings.Fields(string(header))
		if len(fields) != 4 || !objectID(fields[2], idLength) {
			return nil, ErrCorrupt
		}
		entry := treeEntry{mode: fields[0], id: fields[2], name: string(name)}
		switch entry.mode {
		case "040000", "160000":
			kind := "tree"
			if entry.mode == "160000" {
				kind = "commit"
			}
			if fields[1] != kind || fields[3] != "-" {
				return nil, ErrCorrupt
			}
		case "100644", "100755", "120000":
			if fields[1] != "blob" || fields[3] == "" || strings.IndexFunc(fields[3], func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
				return nil, ErrCorrupt
			}
			size, err := strconv.ParseInt(fields[3], 10, 64)
			if err != nil {
				return nil, ErrCorrupt
			}
			entry.size = size
		default:
			return nil, ErrCorrupt
		}
		entries = append(entries, entry)
		data = rest
	}
	slices.SortFunc(entries, func(a, b treeEntry) int { return strings.Compare(a.name, b.name) })
	for i := 1; i < len(entries); i++ {
		if entries[i-1].name == entries[i].name {
			return nil, ErrCorrupt
		}
	}
	return entries, nil
}

func objectID(id string, length int) bool {
	if length == 0 {
		if len(id) != 40 && len(id) != 64 {
			return false
		}
	} else if len(id) != length {
		return false
	}
	for _, b := range []byte(id) {
		if !(b >= '0' && b <= '9' || b >= 'a' && b <= 'f') {
			return false
		}
	}
	return true
}

func cleanPath(name string, root bool) bool {
	return name != "" && len(name) <= 1024 && utf8.ValidString(name) && !strings.ContainsRune(name, 0) &&
		!path.IsAbs(name) && path.Clean(name) == name && name != ".." &&
		!strings.HasPrefix(name, "../") && (root || name != ".")
}

func matchesBlob(body []byte, id string) bool {
	var digest hash.Hash = sha256.New()
	if len(id) == 40 {
		digest = sha1.New()
	}
	// Standard-library hash writes cannot fail. Include Git's blob header, which
	// differs from the raw-content SHA-256 retained separately in Document.
	_, _ = fmt.Fprintf(digest, "blob %d\x00", len(body))
	_, _ = digest.Write(body)
	return fmt.Sprintf("%x", digest.Sum(nil)) == id
}
