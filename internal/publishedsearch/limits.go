package publishedsearch

import "fmt"

// Validate the complete configuration before any catalog/object I/O, including
// quote budgets which Open itself does not otherwise consume. Each component
// retains its own semantic/admission checks when it opens actual input.
func validateLimits(l Limits) error {
	fields := []struct {
		name  string
		value int
	}{
		{"Manifest.MaxEncodedBytes", l.Manifest.MaxEncodedBytes},
		{"Manifest.MaxSources", l.Manifest.MaxSources},
		{"Manifest.MaxEditions", l.Manifest.MaxEditions},
		{"Manifest.MaxCaptures", l.Manifest.MaxCaptures},
		{"Manifest.MaxFiles", l.Manifest.MaxFiles},
		{"Manifest.MaxDocuments", l.Manifest.MaxDocuments},
		{"Manifest.MaxImages", l.Manifest.MaxImages},
		{"Manifest.MaxSourceFiles", l.Manifest.MaxSourceFiles},
		{"Manifest.MaxSourceDocuments", l.Manifest.MaxSourceDocuments},
		{"Manifest.MaxStringBytes", l.Manifest.MaxStringBytes},
		{"Manifest.MaxValueBytes", l.Manifest.MaxValueBytes},
		{"Edition.MaxEncodedBytes", l.Edition.MaxEncodedBytes},
		{"Edition.MaxArticles", l.Edition.MaxArticles},
		{"Edition.MaxArticleBytes", l.Edition.MaxArticleBytes},
		{"Edition.MaxBlocks", l.Edition.MaxBlocks},
		{"Edition.MaxPassages", l.Edition.MaxPassages},
		{"Edition.MaxTerms", l.Edition.MaxTerms},
		{"Edition.MaxPreparedBytes", l.Edition.MaxPreparedBytes},
		{"Edition.MaxKeyBytes", l.Edition.MaxKeyBytes},
		{"Edition.MaxTermBytes", l.Edition.MaxTermBytes},
		{"Edition.MaxVocabulary", l.Edition.MaxVocabulary},
		{"Edition.MaxPostings", l.Edition.MaxPostings},
		{"Article.MaxEncodedBytes", l.Article.MaxEncodedBytes},
		{"Article.MaxBlocks", l.Article.MaxBlocks},
		{"Article.MaxGroups", l.Article.MaxGroups},
		{"Article.MaxStringBytes", l.Article.MaxStringBytes},
		{"Article.MaxValueBytes", l.Article.MaxValueBytes},
		{"Article.MaxGroupDepth", l.Article.MaxGroupDepth},
		{"Article.MaxHTMLBytes", l.Article.MaxHTMLBytes},
	}
	for _, f := range fields {
		if f.value <= 0 {
			return fmt.Errorf("%w: %s must be positive", ErrInvalid, f.name)
		}
	}
	if l.Manifest.MaxObjectBytes <= 0 || l.Manifest.MaxReferencedBytes <= 0 {
		return fmt.Errorf("%w: manifest reference byte limits must be positive", ErrInvalid)
	}
	return nil
}
