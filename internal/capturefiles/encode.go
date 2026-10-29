package capturefiles

import (
	"context"
	"encoding/hex"
	"encoding/json"
)

type fileWire struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
}
type pageWire struct {
	Path string   `json:"path"`
	HTML fileWire `json:"html"`
}
type profileWire struct {
	ID            string   `json:"id"`
	ArticleClass  string   `json:"article_class"`
	OmitClasses   []string `json:"omit_classes"`
	OmitIDs       []string `json:"omit_ids"`
	NoticeClasses []string `json:"notice_classes"`
}
type captureWire struct {
	Source       string      `json:"source"`
	Revision     string      `json:"revision"`
	Directory    string      `json:"directory"`
	Preparation  string      `json:"preparation"`
	Toolchain    string      `json:"toolchain"`
	Recipe       fileWire    `json:"recipe"`
	Dependencies []string    `json:"dependencies"`
	Profile      profileWire `json:"profile"`
	Pages        []pageWire  `json:"pages"`
}
type editionWire struct {
	ID       string        `json:"id"`
	Captures []captureWire `json:"captures"`
}
type descriptionWire struct {
	Format   string        `json:"format"`
	Editions []editionWire `json:"editions"`
}

func wireFile(f File) fileWire {
	return fileWire{Path: f.Path, SHA256: hex.EncodeToString(f.SHA256[:]), Bytes: f.Bytes}
}

// Encode returns owned descriptor bytes; it never reads/writes files or supplies a
// durability/atomic-directory protocol. Producers write and retain stable payloads
// themselves. Nil input lists are encoded as required empty arrays, never null.
// Values/counts are admitted before wire copies/marshal. Encoding work depends on
// escaped JSON output; its allocation may exceed the final encoded-byte ceiling
// before rejection, bounded by admitted metadata. No aliases survive return.
func Encode(ctx context.Context, d Description, l Limits) ([]byte, error) {
	if err := ready(ctx, l); err != nil {
		return nil, err
	}
	if _, err := validate(ctx, d, l); err != nil {
		return nil, err
	}
	w := descriptionWire{Format: Format, Editions: []editionWire{}}
	for _, e := range d.Editions {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		ed := editionWire{ID: e.ID, Captures: []captureWire{}}
		for _, c := range e.Captures {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			p := c.Profile
			cap := captureWire{Source: c.Source, Revision: c.Revision, Directory: c.Directory, Preparation: c.Preparation, Toolchain: c.Toolchain, Recipe: wireFile(c.Recipe), Dependencies: append([]string{}, c.Dependencies...), Profile: profileWire{ID: p.ID, ArticleClass: p.ArticleClass, OmitClasses: append([]string{}, p.OmitClasses...), OmitIDs: append([]string{}, p.OmitIDs...), NoticeClasses: append([]string{}, p.NoticeClasses...)}, Pages: []pageWire{}}
			for _, page := range c.Pages {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
				cap.Pages = append(cap.Pages, pageWire{Path: page.Path, HTML: wireFile(page.HTML)})
			}
			ed.Captures = append(ed.Captures, cap)
		}
		w.Editions = append(w.Editions, ed)
	}
	b, err := json.Marshal(w)
	if err != nil {
		return nil, ErrInvalid
	}
	if len(b) > l.MaxDescriptorBytes {
		return nil, ErrLimit
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return b, nil
}
