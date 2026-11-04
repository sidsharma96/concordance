package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/sidsharma96/concordance/internal/capturefiles"
	"github.com/sidsharma96/concordance/internal/catalog"
	"github.com/sidsharma96/concordance/internal/generationpublish"
	"github.com/sidsharma96/concordance/internal/gitsource"
	"github.com/sidsharma96/concordance/internal/htmlarticle"
	"github.com/sidsharma96/concordance/internal/htmlarticlejson"
	"github.com/sidsharma96/concordance/internal/htmledition"
	"github.com/sidsharma96/concordance/internal/htmlpassage"
	"github.com/sidsharma96/concordance/internal/htmlterms"
	"github.com/sidsharma96/concordance/internal/importjob"
	"github.com/sidsharma96/concordance/internal/preparedimport"
	"github.com/sidsharma96/concordance/internal/textterms"
)

const importUsage = "Usage: concordance import start --capture INPUT --job NEW_JOB --sources PROFILE [flags]\n       concordance import retry --job EXISTING_JOB --sources PROFILE [flags]\n\nStart saves a verified local job then publishes synchronously. Retry uses the same\nsaved key and inputs. Sources PROFILE is trusted local registration; Git is still\nrequired. Configure PGSERVICE, CONCORDANCE_COLLECTION and R2 as for search.\nThe catalog schema and exact storage namespace must already exist. No setup,\nactivation, automatic resume or deletion is performed. Keep the job after errors.\n"

func runImport(parent context.Context, args []string, out io.Writer, getenv func(string) string, connect searchConnector) error {
	if parent == nil || out == nil || getenv == nil || connect == nil || len(args) > 64 {
		return errors.New("use import start or import retry; see import --help")
	}
	if len(args) == 0 || len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		f := flag.NewFlagSet("import", flag.ContinueOnError)
		var o importOptions
		o.flags(f)
		f.String("job", "", "new directory for start; existing saved directory for retry")
		f.String("capture", "", "trusted prepared input directory (start only)")
		f.String("sources", "", "trusted versioned source-registration file")
		return searchHelp(f, out, importUsage)
	}
	mode := args[0]
	if mode != "start" && mode != "retry" {
		return errors.New("use import start or import retry")
	}
	f := flag.NewFlagSet("import "+mode, flag.ContinueOnError)
	f.SetOutput(io.Discard)
	job := f.String("job", "", "new directory for start; existing saved directory for retry")
	capture := f.String("capture", "", "trusted prepared input directory (start only)")
	profile := f.String("sources", "", "trusted versioned source-registration file")
	var o importOptions
	o.flags(f)
	if err := f.Parse(args[1:]); errors.Is(err, flag.ErrHelp) {
		return searchHelp(f, out, importUsage)
	} else if err != nil {
		return errors.New("invalid import flags; use import --help")
	}
	if f.NArg() != 0 || *job == "" || *profile == "" || len(*job) > maxSourceString ||
		len(*capture) > maxSourceString || len(*profile) > maxSourceString ||
		mode == "start" && *capture == "" || mode == "retry" && *capture != "" || !o.valid() || o.object < 2 {
		return errors.New("provide valid import flags and positive limits; use import --help")
	}
	ctx, cancel := context.WithTimeout(parent, o.timeout)
	defer cancel()
	cfg, err := loadSearchConfig(getenv, true)
	if err != nil {
		return err
	}
	sources, err := readSources(ctx, *profile)
	if err != nil {
		return importFailure("read trusted sources failed", *job, err)
	}
	if mode == "start" {
		_, err := importjob.Prepare(ctx, *capture, *job, jobRequest(cfg.collection, sources), o.jobLimits())
		if err != nil {
			return importFailure("save local job failed", *job, err)
		}
	}
	opened, err := importjob.Open(ctx, *job, o.jobLimits())
	if err != nil {
		return importFailure("open saved job failed", *job, err)
	}
	if !matchJob(opened.Saved, cfg.collection, sources) {
		return importFailure("saved collection or source identities disagree with trusted configuration", *job, importjob.ErrMismatch)
	}
	if err := ctx.Err(); err != nil {
		return importFailure("import canceled before connection", *job, err)
	}
	stores, err := connect(ctx, cfg, int64(o.object))
	if err != nil {
		return importFailure("connect import stores failed", *job, err)
	}
	defer stores.close()
	namespace, err := stores.catalog.OpenNamespace(ctx, stores.objects.Binding())
	if err != nil {
		return importFailure("open existing storage namespace failed", *job, err)
	}
	publisher, err := generationpublish.New(stores.catalog, stores.objects, namespace, o.publisherLimits())
	if err != nil {
		return importFailure("configure publisher failed", *job, err)
	}
	importer, err := preparedimport.New(publisher, cfg.collection, sources, o.importerLimits())
	if err != nil {
		return importFailure("configure importer failed", *job, err)
	}
	result, err := importer.ImportPrepared(ctx, opened.Key, opened.CandidateID, opened.Editions)
	if err != nil {
		return importFailure("publication failed or its outcome is uncertain", *job, err)
	}
	if err := ctx.Err(); err != nil {
		return importFailure("publication completed without output acknowledgment", *job, err)
	}
	b := &displayBuffer{max: 64 << 10}
	_, renderErr := fmt.Fprintf(b, "Published generation: %s\nCollection: %s\nJob: %s\nRecovered existing result: %t\nTarget activation remains separate.\n",
		visible(result.Generation.ID), visible(opened.Collection), visible(*job), result.Duplicate)
	if renderErr != nil {
		return importFailure("publication completed but formatting failed", *job, renderErr)
	}
	if err := writeCommand(out, b.body); err != nil {
		return importFailure("publication completed but output failed", *job, err)
	}
	return nil
}

// Do not display native/provider/source diagnostics or reuse search's missing
// worker-socket classification for a missing import file. The original cause is
// retained for errors.Is/As without printing credentials, paths or input bodies.
func importFailure(stage, job string, cause error) error {
	switch {
	case errors.Is(cause, context.Canceled):
		stage += ": canceled"
	case errors.Is(cause, context.DeadlineExceeded):
		stage += ": deadline exceeded"
	case errors.Is(cause, os.ErrExist):
		stage += ": destination already exists; start will not overwrite it"
	case errors.Is(cause, os.ErrNotExist):
		stage += ": required local input is missing"
	case errors.Is(cause, catalog.ErrNotFound):
		stage += ": no namespace for configured physical binding; complete explicit operator setup"
	case errors.Is(cause, catalog.ErrNamespace):
		stage += ": receipt reuse/publication is quarantined or stale"
	case errors.Is(cause, catalog.ErrConflict):
		stage += ": saved key conflicts with captured inputs"
	case errors.Is(cause, importjob.ErrMismatch), errors.Is(cause, capturefiles.ErrMismatch):
		stage += ": saved bytes or expected source identities do not match"
	case errors.Is(cause, preparedimport.ErrMembership):
		stage += ": prepared pages do not match the complete pinned Git selection"
	case errors.Is(cause, importjob.ErrPolicy), errors.Is(cause, catalog.ErrFingerprintVersion):
		stage += ": saved processing policy requires a compatible build"
	case errors.Is(cause, importjob.ErrNotReady):
		stage += ": local job is incomplete or invalid"
	case errors.Is(cause, importjob.ErrLimit), errors.Is(cause, capturefiles.ErrLimit), errors.Is(cause, preparedimport.ErrLimit),
		errors.Is(cause, generationpublish.ErrLimit), errors.Is(cause, gitsource.ErrLimit), errors.Is(cause, htmlarticle.ErrLimit),
		errors.Is(cause, htmlarticlejson.ErrLimit), errors.Is(cause, htmledition.ErrLimit), errors.Is(cause, htmlpassage.ErrLimit),
		errors.Is(cause, htmlterms.ErrLimit), errors.Is(cause, textterms.ErrLimit):
		stage += ": import admission limit exceeded; inspect import --help"
	}
	return &commandError{fmt.Sprintf("%s. Keep job %q and use import retry with that same job if it is ready; do not silently start another key", stage, job), cause}
}
