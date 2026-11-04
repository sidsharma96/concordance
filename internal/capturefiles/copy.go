package capturefiles

import (
	"context"
	"crypto/sha256"
	"fmt"
)

// CopyInfo owns small metadata, no payloads. Original is diagnostic only;
// Descriptor pins the rewritten transport actually emitted. Sources is the unique
// source-ID set in first-appearance order. Copies share its mutable slice.
type CopyInfo struct {
	Original, Descriptor DescriptorRef
	Sources              []string
	PayloadFiles         int
	PayloadBytes         int64
}

// Copy reads and verifies a prepared capture once, then emits its exact owned
// payloads followed by a descriptor with flat transport names. Git/source paths,
// profiles and payload bytes are unchanged. Write borrows bytes unchanged only
// through each synchronous callback; it must not retain or mutate them. No original
// file is reopened after verification. The caller owns destination creation,
// durability, cancellation of its writes and any partial output on error. Copy
// returns zero info on every error. It neither publishes nor marks a job ready.
// Work/memory are bounded by admitted capture metadata/payloads plus encoded
// descriptor storage; this is not streaming or a whole-process heap quota.
func Copy(ctx context.Context, directory string, l Limits, write func(string, []byte) error) (CopyInfo, error) {
	if write == nil {
		return CopyInfo{}, ErrInvalid
	}
	loaded, err := readCapture(ctx, directory, nil, l)
	if err != nil {
		return CopyInfo{}, err
	}
	info := CopyInfo{
		Original: DescriptorRef{SHA256: sha256.Sum256(loaded.descriptor), Bytes: int64(len(loaded.descriptor))},
		Sources:  []string{}, PayloadFiles: len(loaded.files),
	}
	names := make(map[string]string, len(loaded.files))
	for i, file := range loaded.files {
		names[file.Path] = fmt.Sprintf("payload-%08d", i)
		info.PayloadBytes += file.Bytes // Unique total is bounded by admitted reference bytes.
	}
	seen := make(map[string]bool)
	for _, edition := range loaded.description.Editions {
		for i := range edition.Captures {
			capture := &edition.Captures[i]
			if !seen[capture.Source] {
				seen[capture.Source] = true
				info.Sources = append(info.Sources, capture.Source)
			}
			capture.Recipe.Path = names[capture.Recipe.Path]
			for j := range capture.Pages {
				capture.Pages[j].HTML.Path = names[capture.Pages[j].HTML.Path]
			}
		}
	}
	descriptor, err := Encode(ctx, loaded.description, l)
	if err != nil {
		return CopyInfo{}, err
	}
	info.Descriptor = DescriptorRef{SHA256: sha256.Sum256(descriptor), Bytes: int64(len(descriptor))}
	for _, file := range loaded.files {
		if err := ctx.Err(); err != nil {
			return CopyInfo{}, err
		}
		if err := write(names[file.Path], loaded.payloads[file.Path]); err != nil {
			return CopyInfo{}, err
		}
	}
	if err := ctx.Err(); err != nil {
		return CopyInfo{}, err
	}
	if err := write(DescriptorName, descriptor); err != nil {
		return CopyInfo{}, err
	}
	if err := ctx.Err(); err != nil {
		return CopyInfo{}, err
	}
	return info, nil
}
