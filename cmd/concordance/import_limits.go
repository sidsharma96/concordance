package main

import (
	"flag"
	"time"

	"github.com/sidsharma96/concordance/internal/capturefiles"
	gm "github.com/sidsharma96/concordance/internal/generationmanifest"
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

// These are operator admission ceilings, not measured capacity or heap quotas.
// Fixed processing/format limits reject excess rather than split/truncate input.
type importOptions struct {
	timeout                              time.Duration
	input, staged, payload               int64
	object, documents, files, concurrent int
}

func (o *importOptions) flags(f *flag.FlagSet) {
	f.DurationVar(&o.timeout, "timeout", 10*time.Minute, "whole operation deadline, including local save and publication")
	f.Int64Var(&o.input, "max-input-bytes", 512<<20, "aggregate HTML, recipe and acquired Git byte occurrence ceiling")
	f.Int64Var(&o.staged, "max-staged-bytes", 256<<20, "distinct staged bodies plus root byte ceiling")
	f.Int64Var(&o.payload, "max-capture-bytes", 64<<20, "prepared HTML/recipe byte occurrence ceiling")
	f.IntVar(&o.object, "max-object-bytes", 8<<20, "one encoded object or captured file byte ceiling")
	f.IntVar(&o.documents, "max-documents", 1000, "generation logical document occurrence ceiling")
	f.IntVar(&o.files, "max-files", 20000, "generation raw file occurrence ceiling")
	f.IntVar(&o.concurrent, "object-concurrency", 1, "object confirmation chains per call, 1..8")
}

func (o importOptions) valid() bool {
	// Safely reserve limit+1 and arithmetic space on the current architecture.
	capacity := int64(int(^uint(0)>>1)) / 4
	if o.timeout <= 0 || o.concurrent < 1 || o.concurrent > 8 {
		return false
	}
	for _, v := range []int64{o.input, o.staged, o.payload, int64(o.object), int64(o.documents), int64(o.files)} {
		if v <= 0 || v > capacity {
			return false
		}
	}
	return o.files >= o.documents
}

func (o importOptions) jobLimits() importjob.Limits {
	return importjob.Limits{MaxRecordBytes: 1 << 20, MaxSources: maxImportSources,
		Capture: capturefiles.Limits{MaxDescriptorBytes: 8 << 20, MaxEditions: 64, MaxCaptures: 256,
			MaxPages: o.documents, MaxDependencies: o.files, MaxFiles: o.documents + 256,
			MaxStringBytes: maxSourceString, MaxValueBytes: 8 << 20,
			MaxFileBytes: int64(o.object), MaxPayloadBytes: o.payload}}
}

func (o importOptions) importerLimits() preparedimport.Limits {
	a := htmlarticlejson.Limits{MaxEncodedBytes: o.object, MaxBlocks: 20000, MaxGroups: 40000, MaxStringBytes: 1 << 20, MaxValueBytes: o.object, MaxGroupDepth: 512, MaxHTMLBytes: o.object}
	e := htmledition.Limits{MaxEncodedBytes: o.object, MaxArticles: o.documents, MaxArticleBytes: 64 << 20,
		MaxBlocks: 200000, MaxPassages: 20000, MaxTerms: 1000000, MaxPreparedBytes: 16 << 20,
		MaxKeyBytes: 2 << 20, MaxTermBytes: 64 << 10, MaxVocabulary: 20000, MaxPostings: 1000000}
	p := htmlterms.Limits{
		Passages: htmlpassage.Limits{MaxBlocks: 20000, MaxGroups: 40000, MaxInputBytes: 8 << 20,
			MaxPassages: 20000, MaxBodyBytes: 8192, MaxReferences: 200000, MaxContextBytes: 8 << 20},
		Field:        textterms.Limits{MaxInputBytes: 64 << 10, MaxTerms: 10000, MaxTermBytes: 64 << 10, MaxOutputBytes: 128 << 10},
		MaxViewBytes: 8 << 20, MaxTerms: 200000, MaxOutputBytes: 8 << 20}
	return preparedimport.Limits{
		MaxInputBytes: o.input, MaxStagedBytes: o.staged, MaxObjects: o.files + o.documents + 256, MaxHashBytes: 16 << 20,
		Manifest: gm.Limits{MaxEncodedBytes: o.object, MaxSources: maxImportSources, MaxEditions: 64, MaxCaptures: 256,
			MaxFiles: o.files, MaxDocuments: o.documents, MaxImages: o.files, MaxSourceFiles: o.files,
			MaxSourceDocuments: o.documents, MaxStringBytes: maxSourceString, MaxValueBytes: 8 << 20,
			MaxObjectBytes: int64(o.object), MaxReferencedBytes: o.input},
		Git:   gitsource.Limits{MaxEntries: o.documents, MaxListingBytes: 4 << 20, MaxDocumentBytes: int64(o.object), MaxTotalBytes: max(128<<20, int64(o.object))},
		Files: gitsource.FileLimits{MaxFiles: o.files, MaxListingBytes: 8 << 20, MaxFileBytes: int64(o.object), MaxTotalBytes: max(128<<20, int64(o.object))},
		Extract: htmlarticle.Limits{MaxInputBytes: o.object, MaxTokenBytes: min(128<<10, o.object/2), MaxTokens: 200000, MaxNodes: 200000,
			MaxDepth: 512, MaxBlocks: 20000, MaxGroups: 40000, MaxBlockBytes: 1 << 20, MaxOutputBytes: 8 << 20},
		Edition: htmledition.BuildLimits{File: e, Article: a, Preparation: p}}
}

func (o importOptions) publisherLimits() generationpublish.Limits {
	l := o.importerLimits()
	return generationpublish.Limits{MaxObjects: l.MaxObjects, MaxBatchObjects: 32, MaxConcurrentObjects: o.concurrent,
		MaxInputBytes: l.MaxStagedBytes, MaxBatchBytes: int64(o.object), Manifest: l.Manifest, Edition: l.Edition.File, Article: l.Edition.Article}
}
