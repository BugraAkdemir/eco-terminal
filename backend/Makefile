TAGS=sqlite_fts5 sqlite_rtree
.PHONY: build vet test run
build:
	go build -tags "$(TAGS)" -o bin/server ./cmd/server
vet:
	go vet -tags "$(TAGS)" ./...
test:
	go test -tags "$(TAGS)" -race ./...
run:
	go run -tags "$(TAGS)" ./cmd/server
