package preparedimport

import "fmt"

// Validate configuration before inspecting caller input or allocating source maps.
// Keep component names here so errors identify the budget the operator must fix.
func validateLimits(l Limits) error {
	for _, limit := range []struct {
		name  string
		value int64
	}{
		{"MaxInputBytes", int64(l.MaxInputBytes)},
		{"MaxStagedBytes", int64(l.MaxStagedBytes)},
		{"MaxObjects", int64(l.MaxObjects)},
		{"MaxHashBytes", int64(l.MaxHashBytes)},
		{"Manifest.MaxEncodedBytes", int64(l.Manifest.MaxEncodedBytes)},
		{"Manifest.MaxSources", int64(l.Manifest.MaxSources)},
		{"Manifest.MaxEditions", int64(l.Manifest.MaxEditions)},
		{"Manifest.MaxCaptures", int64(l.Manifest.MaxCaptures)},
		{"Manifest.MaxFiles", int64(l.Manifest.MaxFiles)},
		{"Manifest.MaxDocuments", int64(l.Manifest.MaxDocuments)},
		{"Manifest.MaxImages", int64(l.Manifest.MaxImages)},
		{"Manifest.MaxSourceFiles", int64(l.Manifest.MaxSourceFiles)},
		{"Manifest.MaxSourceDocuments", int64(l.Manifest.MaxSourceDocuments)},
		{"Manifest.MaxStringBytes", int64(l.Manifest.MaxStringBytes)},
		{"Manifest.MaxValueBytes", int64(l.Manifest.MaxValueBytes)},
		{"Manifest.MaxObjectBytes", int64(l.Manifest.MaxObjectBytes)},
		{"Manifest.MaxReferencedBytes", int64(l.Manifest.MaxReferencedBytes)},
		{"Git.MaxEntries", int64(l.Git.MaxEntries)},
		{"Git.MaxListingBytes", int64(l.Git.MaxListingBytes)},
		{"Git.MaxDocumentBytes", int64(l.Git.MaxDocumentBytes)},
		{"Git.MaxTotalBytes", int64(l.Git.MaxTotalBytes)},
		{"Files.MaxFiles", int64(l.Files.MaxFiles)},
		{"Files.MaxListingBytes", int64(l.Files.MaxListingBytes)},
		{"Files.MaxFileBytes", int64(l.Files.MaxFileBytes)},
		{"Files.MaxTotalBytes", int64(l.Files.MaxTotalBytes)},
		{"Extract.MaxInputBytes", int64(l.Extract.MaxInputBytes)},
		{"Extract.MaxTokenBytes", int64(l.Extract.MaxTokenBytes)},
		{"Extract.MaxTokens", int64(l.Extract.MaxTokens)},
		{"Extract.MaxNodes", int64(l.Extract.MaxNodes)},
		{"Extract.MaxDepth", int64(l.Extract.MaxDepth)},
		{"Extract.MaxBlocks", int64(l.Extract.MaxBlocks)},
		{"Extract.MaxGroups", int64(l.Extract.MaxGroups)},
		{"Extract.MaxBlockBytes", int64(l.Extract.MaxBlockBytes)},
		{"Extract.MaxOutputBytes", int64(l.Extract.MaxOutputBytes)},
		{"Edition.File.MaxEncodedBytes", int64(l.Edition.File.MaxEncodedBytes)},
		{"Edition.File.MaxArticles", int64(l.Edition.File.MaxArticles)},
		{"Edition.File.MaxArticleBytes", int64(l.Edition.File.MaxArticleBytes)},
		{"Edition.File.MaxBlocks", int64(l.Edition.File.MaxBlocks)},
		{"Edition.File.MaxPassages", int64(l.Edition.File.MaxPassages)},
		{"Edition.File.MaxTerms", int64(l.Edition.File.MaxTerms)},
		{"Edition.File.MaxPreparedBytes", int64(l.Edition.File.MaxPreparedBytes)},
		{"Edition.File.MaxKeyBytes", int64(l.Edition.File.MaxKeyBytes)},
		{"Edition.File.MaxTermBytes", int64(l.Edition.File.MaxTermBytes)},
		{"Edition.File.MaxVocabulary", int64(l.Edition.File.MaxVocabulary)},
		{"Edition.File.MaxPostings", int64(l.Edition.File.MaxPostings)},
		{"Edition.Article.MaxEncodedBytes", int64(l.Edition.Article.MaxEncodedBytes)},
		{"Edition.Article.MaxBlocks", int64(l.Edition.Article.MaxBlocks)},
		{"Edition.Article.MaxGroups", int64(l.Edition.Article.MaxGroups)},
		{"Edition.Article.MaxStringBytes", int64(l.Edition.Article.MaxStringBytes)},
		{"Edition.Article.MaxValueBytes", int64(l.Edition.Article.MaxValueBytes)},
		{"Edition.Article.MaxGroupDepth", int64(l.Edition.Article.MaxGroupDepth)},
		{"Edition.Article.MaxHTMLBytes", int64(l.Edition.Article.MaxHTMLBytes)},
		{"Edition.Preparation.Passages.MaxBlocks", int64(l.Edition.Preparation.Passages.MaxBlocks)},
		{"Edition.Preparation.Passages.MaxGroups", int64(l.Edition.Preparation.Passages.MaxGroups)},
		{"Edition.Preparation.Passages.MaxInputBytes", int64(l.Edition.Preparation.Passages.MaxInputBytes)},
		{"Edition.Preparation.Passages.MaxPassages", int64(l.Edition.Preparation.Passages.MaxPassages)},
		{"Edition.Preparation.Passages.MaxBodyBytes", int64(l.Edition.Preparation.Passages.MaxBodyBytes)},
		{"Edition.Preparation.Passages.MaxReferences", int64(l.Edition.Preparation.Passages.MaxReferences)},
		{"Edition.Preparation.Passages.MaxContextBytes", int64(l.Edition.Preparation.Passages.MaxContextBytes)},
		{"Edition.Preparation.Field.MaxInputBytes", int64(l.Edition.Preparation.Field.MaxInputBytes)},
		{"Edition.Preparation.Field.MaxTerms", int64(l.Edition.Preparation.Field.MaxTerms)},
		{"Edition.Preparation.Field.MaxTermBytes", int64(l.Edition.Preparation.Field.MaxTermBytes)},
		{"Edition.Preparation.Field.MaxOutputBytes", int64(l.Edition.Preparation.Field.MaxOutputBytes)},
		{"Edition.Preparation.MaxViewBytes", int64(l.Edition.Preparation.MaxViewBytes)},
		{"Edition.Preparation.MaxTerms", int64(l.Edition.Preparation.MaxTerms)},
		{"Edition.Preparation.MaxOutputBytes", int64(l.Edition.Preparation.MaxOutputBytes)},
	} {
		if limit.value <= 0 {
			return fmt.Errorf("%w: %s must be positive", ErrInvalid, limit.name)
		}
	}
	if l.Git.MaxTotalBytes < l.Git.MaxDocumentBytes {
		return fmt.Errorf("%w: Git.MaxTotalBytes must cover Git.MaxDocumentBytes", ErrInvalid)
	}
	if l.Files.MaxTotalBytes < l.Files.MaxFileBytes {
		return fmt.Errorf("%w: Files.MaxTotalBytes must cover Files.MaxFileBytes", ErrInvalid)
	}
	if l.Extract.MaxTokenBytes >= l.Extract.MaxInputBytes {
		return fmt.Errorf("%w: Extract.MaxTokenBytes must be less than Extract.MaxInputBytes", ErrInvalid)
	}
	if l.Extract.MaxDepth > 512 {
		return fmt.Errorf("%w: Extract.MaxDepth must be at most 512", ErrInvalid)
	}
	if l.Extract.MaxOutputBytes < l.Extract.MaxBlockBytes {
		return fmt.Errorf("%w: Extract.MaxOutputBytes must cover Extract.MaxBlockBytes", ErrInvalid)
	}
	return nil
}
