.PHONY: test
test:
	@out=$$(gofmt -l cmd internal); if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi
	go mod tidy -diff
	go vet ./...
	go test -race -count=1 ./...

.PHONY: build
build:
	go build -o sitrep ./cmd/sitrep

# Builds the program beside the installed one and lets it install itself, as /sitrep:setup does.
.PHONY: install
install:
	mkdir -p "$$HOME/.claude/sitrep"
	go build -o "$$HOME/.claude/sitrep/sitrep.new" -mod=readonly -buildvcs=false ./cmd/sitrep
	"$$HOME/.claude/sitrep/sitrep.new" setup
