all: hooks

SOURCES := $(shell find . -name '*.go')

.PHONY: hooks
hooks: bin/hotdog-cc-hook

bin/hotdog-cc-hook: $(SOURCES) go.mod go.sum
	go build -o bin/hotdog-cc-hook ./cmd/hotdog-cc-hook

clean:
	rm -rf bin