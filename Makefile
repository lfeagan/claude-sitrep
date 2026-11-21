.PHONY: test
test:
	@out=$$(gofmt -l cmd internal); if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi
	go mod tidy -diff
	go vet ./...
	go test -race -count=1 ./...

# The differential tests compare with the last bash version, the git tag bash-final. They skip without git, jq, perl and the tag,
# and fail when SITREP_REQUIRE_ORACLE is set, as it is in CI.
.PHONY: build
build:
	go build -o sitrep ./cmd/sitrep
