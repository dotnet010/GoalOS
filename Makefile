.PHONY: build test lint race deadcode clean install-plugin release

build:
	go build ./...

test:
	go test -count=1 -timeout 120s ./...

race:
	go test -count=1 -timeout 120s -race ./...

lint:
	go vet ./...

deadcode:
	@which staticcheck > /dev/null 2>&1 && staticcheck ./... || echo "install staticcheck: go install honnef.co/go/tools/cmd/staticcheck@latest"

all: lint race deadcode test build

install-plugin:
	@mkdir -p ~/.goalos/plugins/capability/websearch
	CGO_ENABLED=0 go build -o ~/.goalos/plugins/capability/websearch/plugin-websearch ./cmd/plugin-websearch/
	cp plugins/capability/websearch/plugin.json ~/.goalos/plugins/capability/websearch/
	@echo "Plugin installed to ~/.goalos/plugins/capability/websearch/"

daemon:
	go build -o goalos-daemon ./cmd/goalos/

release: test
	CGO_ENABLED=0 go build -o goalos-daemon ./cmd/goalos/
	CGO_ENABLED=0 go build -o goalos ./cmd/goalos-cli/

clean:
	rm -f goalos-daemon goalos goalos-cli plugin-websearch
