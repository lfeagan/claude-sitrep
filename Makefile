SCRIPTS := bin/sitrep hooks/sitrep-hook.sh statusline/statusline.sh lib/conf.sh install.sh uninstall.sh setup.sh tests/run.sh

.PHONY: lint
lint:
	@for f in $(SCRIPTS); do bash -n "$$f" || exit 1; done
	shellcheck -x $(SCRIPTS)

.PHONY: test
test:
	bash tests/run.sh

# The optional Go dashboard (needs Go 1.26 or newer). `make lint test` stays Go-free.
.PHONY: tui
tui:
	cd tui && go build -o ../bin/sitrep-tui .

.PHONY: tui-test
tui-test:
	cd tui && test -z "$$(gofmt -l .)" && go vet ./... && go test -race -count=1 ./...
	cd tui && go build -o ../bin/sitrep-tui .
	h=$$(mktemp -d); HOME=$$h SITREP_BIN=$(CURDIR)/bin/sitrep bin/sitrep-tui --once | grep -q Watch; r=$$?; rmdir "$$h"; exit $$r

# The Go port (see docs/GO-PORT.md). The differential tests need git, jq and perl, and skip without the bash-final tag.
.PHONY: go-test
go-test:
	test -z "$$(gofmt -l cmd internal)" && go vet ./... && go test -race -count=1 ./...
