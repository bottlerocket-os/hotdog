all: hooks

SOURCES := $(shell find . -name '*.go')

.PHONY: hooks
hooks: bin/hotdog-cc-hook bin/hotdog-poststart-hook bin/hotdog-hotpatch

bin/hotdog-cc-hook: $(SOURCES) go.mod go.sum
	go build -mod=readonly -o bin/hotdog-cc-hook ./cmd/hotdog-cc-hook

bin/hotdog-poststart-hook: $(SOURCES) go.mod go.sum
	go build -mod=readonly -o bin/hotdog-poststart-hook ./cmd/hotdog-poststart-hook

bin/hotdog-hotpatch: $(SOURCES) go.mod go.sum
	CGO_ENABLED=0 go build -mod=readonly -installsuffix cgo -a -ldflags "-s" -o bin/hotdog-hotpatch ./cmd/hotdog-hotpatch

install: hooks
	install -D bin/hotdog-cc-hook /usr/libexec/hotdog/hotdog-cc-hook
	install -D bin/hotdog-poststart-hook /usr/libexec/hotdog/hotdog-poststart-hook
	install -D bin/hotdog-hotpatch /usr/share/hotdog/hotdog-hotpatch

clean:
	rm -rf bin