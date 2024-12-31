build:
	go build app/tui/notes_search.go

dev-build:
	CGO_ENABLED=0 go build app/tui/notes_search.go

install:
	go install app/tui/notes_search.go
