package main

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/sidsharma96/concordance/tools/internal/textnav"
)

const maxPinBytes = 1 << 20

// readPins reads the existing RFC inventory format. This source-specific filename
// convention stays here; the shared text reader knows neither RFCs nor sections.
func readPins(path, exclusions string) ([]textnav.Pin, error) {
	before, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() {
		return nil, textnav.ErrInvalid
	}
	if before.Size() > maxPinBytes {
		return nil, textnav.ErrLimit
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil {
		return nil, errors.Join(err, f.Close())
	}
	if !info.Mode().IsRegular() {
		return nil, errors.Join(textnav.ErrInvalid, f.Close())
	}
	if info.Size() > maxPinBytes {
		return nil, errors.Join(textnav.ErrLimit, f.Close())
	}
	body, readErr := io.ReadAll(io.LimitReader(f, maxPinBytes+1))
	if err := errors.Join(readErr, f.Close()); err != nil {
		return nil, err
	}
	if len(body) > maxPinBytes {
		return nil, textnav.ErrLimit
	}
	return parsePins(body, exclusions)
}

func parsePins(body []byte, exclusions string) ([]textnav.Pin, error) {
	exclude := make(map[string]bool)
	if exclusions != "" {
		for _, id := range strings.Split(exclusions, ",") {
			if !rfcID(id) || exclude[id] {
				return nil, textnav.ErrInvalid
			}
			exclude[id] = true
		}
	}
	seen := make(map[string]bool)
	pins := make([]textnav.Pin, 0)
	// Scanner's default token cap is overridden deliberately: pin records are
	// short metadata, never document bodies. Err catches an overlong final line.
	scanner := bufio.NewScanner(bytes.NewReader(body))
	scanner.Buffer(make([]byte, 256), 256)
	for scanner.Scan() {
		fields := strings.Split(scanner.Text(), " ")
		if len(fields) != 2 || !rfcID(fields[0]) || seen[fields[0]] {
			return nil, textnav.ErrInvalid
		}
		if len(seen) >= 1000 {
			return nil, textnav.ErrLimit
		}
		h, err := hex.DecodeString(fields[1])
		if err != nil || len(h) != 32 || hex.EncodeToString(h) != fields[1] {
			return nil, textnav.ErrInvalid
		}
		seen[fields[0]] = true
		if exclude[fields[0]] {
			continue
		}
		pin := textnav.Pin{ID: fields[0], Path: fields[0] + ".txt"}
		copy(pin.SHA256[:], h)
		pins = append(pins, pin)
	}
	if err := scanner.Err(); err != nil {
		return nil, errors.New("invalid or oversized pin record")
	}
	for id := range exclude {
		if !seen[id] {
			return nil, errors.New("excluded RFC is absent from pin inventory")
		}
	}
	if len(seen) == 0 || len(pins) == 0 {
		return nil, textnav.ErrInvalid
	}
	return pins, nil
}

func rfcID(id string) bool {
	if !strings.HasPrefix(id, "rfc") || len(id) < 4 || id[3] == '0' {
		return false
	}
	n, err := strconv.ParseUint(id[3:], 10, 32)
	return err == nil && n > 0 && "rfc"+strconv.FormatUint(n, 10) == id
}
