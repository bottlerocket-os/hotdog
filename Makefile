all: hooks

SOURCES := $(shell find . -name '*.go')

.PHONY: hooks
hooks: bin/hotdog-cc-hook bin/hotdog-poststart-hook bin/hotdog-poststop-hook

bin/hotdog-cc-hook: $(SOURCES) go.mod go.sum
	go build -o bin/hotdog-cc-hook ./cmd/hotdog-cc-hook

bin/hotdog-poststart-hook: $(SOURCES) go.mod go.sum
	go build -o bin/hotdog-poststart-hook ./cmd/hotdog-poststart-hook

bin/hotdog-poststop-hook: $(SOURCES) go.mod go.sum
	go build -o bin/hotdog-poststop-hook ./cmd/hotdog-poststop-hook

install: hooks
	install bin/hotdog-cc-hook /usr/local/bin
	install bin/hotdog-poststart-hook /usr/local/bin
	install bin/hotdog-poststop-hook /usr/local/bin

clean:
	rm -rf bin