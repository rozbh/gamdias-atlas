PREFIX ?= $(HOME)/.local

.PHONY: build test check install udev clean

build: ## build bin/atlas
	go build -trimpath -ldflags="-s -w" -o bin/atlas ./cmd/atlas

test: ## run the tests
	go test -race ./...

check: ## formatting and static checks
	@test -z "$$(gofmt -l cmd internal)" || { echo "run gofmt on:"; gofmt -l cmd internal; exit 1; }
	go vet ./...

install: build ## copy atlas to $(PREFIX)/bin
	install -Dm755 bin/atlas $(PREFIX)/bin/atlas

udev: ## let your user open the display; run as root: sudo make udev
	install -Dm644 udev/60-gamdias-atlas.rules /etc/udev/rules.d/60-gamdias-atlas.rules
	udevadm control --reload
	udevadm trigger

clean: ## remove everything that was built
	rm -rf bin
	go clean -testcache
