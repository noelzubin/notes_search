package search

type DocumentMatch struct {
	Path    string
	Content string
}

type SearchResult struct {
	Err  error
	Hits []DocumentMatch
}

// The indexer that indexes all the notes and searches them.
type NotesIndexer interface {
	IndexNotes()                      // Index all the notes.
	Search(query string) SearchResult // Search the index for the given query.
	OpenIndex()                       // Search the index for the given query.
	CloseIndex()                      // Search the index for the given query.
}
