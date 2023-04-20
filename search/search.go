package search

type DocumentMatch struct {
	Path    string
	Content string
}

type SearchResult struct {
	Hits []DocumentMatch
}

// The indexer that indexes all the notes and searches them.
type NotesIndexer interface {
	IndexNotes()
	Search(query string) SearchResult
}
