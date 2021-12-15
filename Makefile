all: hooks

SOURCES := $(shell find . -name '*.go')

.PHONY: hooks
hooks: bin/hotdog-cc-hook bin/hotdog-poststart-hook bin/hotdog-poststop-hook bin/hotdog-hotpatch

bin/hotdog-cc-hook: $(SOURCES) go.mod go.sum
	go build -mod=readonly -o bin/hotdog-cc-hook ./cmd/hotdog-cc-hook

bin/hotdog-poststart-hook: $(SOURCES) go.mod go.sum
	go build -mod=readonly -o bin/hotdog-poststart-hook ./cmd/hotdog-poststart-hook

bin/hotdog-poststop-hook: $(SOURCES) go.mod go.sum
	go build -mod=readonly -o bin/hotdog-poststop-hook ./cmd/hotdog-poststop-hook

bin/hotdog-hotpatch: $(SOURCES) go.mod go.sum
	CGO_ENABLED=0 go build -mod=readonly -installsuffix cgo -a -ldflags "-s" -o bin/hotdog-hotpatch ./cmd/hotdog-hotpatch

install: hooks
	install bin/hotdog-cc-hook /usr/local/bin
	install bin/hotdog-poststart-hook /usr/local/bin
	install bin/hotdog-poststop-hook /usr/local/bin
	install -D bin/hotdog-hotpatch /usr/libexec/hotdog

clean:
	rm -rf bin