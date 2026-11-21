SCRIPTS := bin/sitrep hooks/sitrep-hook.sh statusline/statusline.sh lib/conf.sh install.sh uninstall.sh setup.sh tests/run.sh

.PHONY: lint
lint:
	@for f in $(SCRIPTS); do bash -n "$$f" || exit 1; done
	shellcheck -x $(SCRIPTS)

.PHONY: test
test:
	bash tests/run.sh

# The Go port (see docs/GO-PORT.md). The differential tests need git, jq, perl and the bash-final tag; they skip without them, and fail when CI is set.
.PHONY: go-test
go-test:
	@out=$$(gofmt -l cmd internal); if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi
	go mod tidy -diff
	go vet ./...
	go test -race -count=1 ./...
