package bleve_indexer

import (
	"encoding/json"
	"io/fs"
	"io/ioutil"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/blevesearch/bleve/v2"
	"github.com/noelzubin/notes_search/search"
	"github.com/noelzubin/notes_search/utils"
	"github.com/samber/lo"

	_ "github.com/blevesearch/bleve/v2/config"
	bleveSearch "github.com/blevesearch/bleve/v2/search"
)

// bleveIndexer is the implmentation of the SearchIndexer
// interface which uses bleve index.
type bleveIndexer struct {
	notesRoot  string
	extensions []string
	index      bleve.Index
	indexPath  string
}

// returns where index and metadata will be stored on disk.
func getDataPath() string {
	dir, _ := os.UserCacheDir()
	return path.Join(dir, "/notes_search")
}

// Get path to the index
func getIndexPath() string {
	return path.Join(getDataPath(), "/index.bleve")
}

// Get path to the fileinfos.json file
func getFileInfosPath() string {
	return path.Join(getDataPath(), "/fileinfos.json")
}

// NewBleveIndexer returns a new SearchIndexer
func NewBleveIndexer(config *utils.Config) (bleveIndexer, error) {
	if err := os.MkdirAll(getDataPath(), 0700); err != nil {
		return bleveIndexer{}, err
	}

	index_path := getIndexPath()
	index, err := GetIndex(index_path)
	if err != nil {
		return bleveIndexer{}, err
	}

	return bleveIndexer{config.RootPath, config.Extensions, index, index_path}, nil
}

func (s *bleveIndexer) OpenIndex() {
	s.index, _ = GetIndex(s.indexPath)
}

func (s *bleveIndexer) CloseIndex() {
	s.index.Close()
}

// Reindex all the notes.
//
// It compares all the file in the rootPath with the ones in the metadata file.
// If the file is new or modified, it is indexed. If the file is deleted,
// it is removed from the index.
func (s *bleveIndexer) IndexNotes() {
	old, err := readFileInfos(getFileInfosPath())
	if err == fs.ErrNotExist {
		old = make([]FileInfo, 0)
	}

	currentPaths, _ := getListOfNotes(s.notesRoot, s.extensions)

	current := lo.Map(currentPaths, func(path string, _ int) FileInfo {
		fileInfo, _ := getFileInfoForFile(path)
		return fileInfo
	})

	deleted, modified, created := compareFileInfos(old, current)
	toIndex := append(modified, created...)

	var wg sync.WaitGroup

	wg.Add(len(deleted) + len(toIndex))

	for _, fi := range deleted {
		go func(fi FileInfo) {
			defer wg.Done()
			s.index.Delete(fi.Path)
		}(fi)
	}

	for _, fi := range toIndex {
		go func(fi FileInfo) {
			defer wg.Done()
			body, _ := os.ReadFile(fi.Path)
			s.index.Index(fi.Path, Note{Path: fi.Path, Body: string(body), ModTime: fi.ModTime})
		}(fi)
	}

	wg.Wait()

	err = StoreFileInfos(getFileInfosPath(), current)
}

// Search searches the index for the given query.
// If the length of the query is less than 3, it returns all the notes.
func (s *bleveIndexer) Search(qry string) search.SearchResult {
	query := strings.Trim(qry, " ")

	queryLen := len(query)
	if queryLen > 0 && query[queryLen-1] != ' ' {
		query = query + "*"
	}
	bleveQuery := bleve.NewQueryStringQuery(query)
	searchRequest := bleve.NewSearchRequest(bleveQuery)
	searchRequest.Highlight = bleve.NewHighlightWithStyle("ansi")

	if len(query) < 3 {
		searchRequest = bleve.NewSearchRequest(bleve.NewMatchAllQuery())
		searchRequest.SortBy([]string{"-ModTime"})
	}

	searchRequest.Size = 100
	searchResult, err := s.index.Search(searchRequest)
	if err != nil {
		log.Fatal(err)
	}

	var getFragment = func(hit *bleveSearch.DocumentMatch) string {
		content := "..."
		body := hit.Fragments["Body"]
		if body != nil {
			return body[0]
		}
		return content
	}

	result := search.SearchResult{
		Hits: lo.Map(searchResult.Hits, func(hit *bleveSearch.DocumentMatch, _ int) search.DocumentMatch {
			return search.DocumentMatch{
				Path:    hit.ID,
				Content: getFragment(hit),
			}
		}),
	}

	return result
}

// GetIndex returns the index if it exists or creates a new one if it doesn't.
func GetIndex(path string) (bleve.Index, error) {
	index, err := bleve.Open(path)

	if err == bleve.ErrorIndexPathDoesNotExist {
		mapping := bleve.NewIndexMapping()
		index, err = bleve.New(path, mapping)
	}

	if err == nil {
		return index, nil
	}

	mapping := bleve.NewIndexMapping()
	index, err = bleve.New(path, mapping)
	return index, err
}

// getListOfNotes returns a list of all the notes in the given directory
func getListOfNotes(src string, extensions []string) (paths []string, err error) {
	return glob(src, func(path string) bool {
		ext := filepath.Ext(path)

		log.Println("exetnsions to filter by ")
		for _, e := range extensions {
			log.Println(e)
		}
		log.Println("-------")
		return lo.Contains(extensions, ext)
	}), nil
}

// FileInfo contains the path and the last modified time of a file
// This is what is stored in the metadata file
type FileInfo struct {
	Path    string    // Path to the file
	ModTime time.Time // Last modified time
}

// GetFileInfoForFile returns the FileInfo for the given file
func getFileInfoForFile(path string) (fi FileInfo, err error) {
	info, err := os.Stat(path)
	if err != nil {
		return FileInfo{}, err
	}
	return FileInfo{Path: path, ModTime: info.ModTime()}, nil
}

// storeFileInfos stores the given FileInfos in the given path
func StoreFileInfos(path string, fi []FileInfo) (err error) {
	file, err := os.Create(path)

	if err != nil {
		return err
	}

	data, err := json.Marshal(fi)

	if err != nil {
		return err
	}

	file.Write(data)

	return nil
}

// readFileInfos reads the FileInfos from the given path
func readFileInfos(path string) (fi []FileInfo, err error) {
	file, err := os.Open(path)

	if err != nil {
		return nil, err
	}

	data, err := ioutil.ReadAll(file)

	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(data, &fi)

	if err != nil {
		return nil, err
	}

	return fi, nil
}

// compareFileInfos compares the old and current FileInfos and returns the deleted, modified and created FileInfos
func compareFileInfos(old, current []FileInfo) (deleted, modified, created []FileInfo) {

	deleted = make([]FileInfo, 0)
	created = make([]FileInfo, 0)
	modified = make([]FileInfo, 0)

	for _, f1 := range old {
		found := false
		for _, f2 := range current {
			if f1.Path == f2.Path {
				found = true
				if f1.ModTime != f2.ModTime {
					modified = append(modified, f1)
				}
			}
		}
		if !found {
			deleted = append(deleted, f1)
		}
	}

	for _, f2 := range current {
		found := false
		for _, f1 := range old {
			if f2.Path == f1.Path {
				found = true
			}
		}
		if !found {
			created = append(created, f2)
		}
	}

	return deleted, modified, created
}

// Note is the struct that is indexed
type Note struct {
	Path    string
	Body    string
	ModTime time.Time
}

// Custom glob function because inbuild function doesn't support recursive globbing correctly
func glob(root string, fn func(string) bool) []string {
	var matches []string
	filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if fn(path) {
			matches = append(matches, path)
		}
		return nil
	})
	return matches
}
