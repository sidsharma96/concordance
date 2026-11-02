package main

import (
	"context"
	"errors"
	"flag"
	"io"
	"time"

	"github.com/sidsharma96/concordance/internal/catalog"
	"github.com/sidsharma96/concordance/internal/searchtarget"
)

func runTarget(parent context.Context, args []string, out io.Writer, getenv func(string) string, connect searchConnector) error {
	if parent == nil || out == nil || getenv == nil || connect == nil || len(args) > 64 {
		return errors.New("use target show NAME or target activate --help")
	}
	if len(args) == 0 || (len(args) == 1 && (args[0] == "--help" || args[0] == "-h" || args[0] == "help")) {
		return writeCommand(out, []byte("Usage: concordance target show [flags] NAME\n       concordance target activate [flags] NAME\nUse target activate --help for generation, edition and revision flags.\n"))
	}
	mode := args[0]
	if mode != "show" && mode != "activate" {
		return errors.New("use target show NAME or target activate --help")
	}
	f := flag.NewFlagSet("target "+mode, flag.ContinueOnError)
	f.SetOutput(io.Discard)
	var read readOptions
	var generation, edition *string
	var expected *int64
	if mode == "activate" {
		read.flags(f)
		generation = f.String("generation", "", "published generation selected explicitly by the operator")
		edition = f.String("edition", "", "exact default edition")
		expected = f.Int64("expected-revision", 0, "expected target revision; zero creates only if absent")
	} else {
		f.DurationVar(&read.timeout, "timeout", time.Minute, "catalog lookup deadline")
	}
	if err := f.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return searchHelp(f, out, "Usage: concordance target "+mode+" [flags] NAME")
		}
		return commandFailure("invalid target flags; use target "+mode+" --help", err)
	}
	if f.NArg() != 1 || !validName(f.Arg(0)) || read.timeout <= 0 {
		return errors.New("provide one target name and positive timeout")
	}
	if mode == "activate" && !read.valid() {
		return errors.New("provide positive reader limits")
	}
	ctx, cancel := context.WithTimeout(parent, read.timeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return commandFailure("target operation canceled", err)
	}
	cfg, err := loadSearchConfig(getenv, mode == "activate")
	if err != nil {
		return err
	}
	var update catalog.TargetUpdate
	if mode == "activate" {
		update = catalog.TargetUpdate{Name: f.Arg(0), Value: catalog.TargetValue{Collection: cfg.collection, Generation: *generation, DefaultEdition: *edition}, ExpectedRevision: *expected, ExpectedCollection: cfg.collection}
		if err := update.Validate(); err != nil {
			return commandFailure("provide valid generation, edition and expected revision", err)
		}
	}
	maxBytes := int64(1)
	if mode == "activate" {
		maxBytes = int64(max(read.manifest, read.index, read.article))
	}
	stores, err := connect(ctx, cfg, maxBytes)
	if err != nil {
		return err
	}
	defer stores.close()
	var target catalog.Target
	if mode == "activate" {
		target, err = searchtarget.Activate(ctx, stores.catalog, stores.objects, update, read.limits())
	} else {
		target, err = stores.catalog.ResolveTarget(ctx, f.Arg(0))
		if err == nil && target.Value.Collection != cfg.collection {
			return errors.New("target is outside CONCORDANCE_COLLECTION")
		}
	}
	if err != nil {
		return commandFailure("target "+mode+" failed", err)
	}
	body, err := renderTarget(target)
	if err != nil {
		return commandFailure("format target failed", err)
	}
	if err := ctx.Err(); err != nil {
		return commandFailure("target operation finished without an output acknowledgment; inspect target show", err)
	}
	err = writeCommand(out, body)
	if err != nil && mode == "activate" {
		return commandFailure("target may have changed; inspect target show after output failure", err)
	}
	return err
}
