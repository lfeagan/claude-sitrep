.PHONY: test
test:
	@out=$$(gofmt -l cmd internal); if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi
	go mod tidy -diff
	go vet ./...
	go test -race -count=1 ./...

# Redraws the pictures of the README in docs/img from the made-up demo sessions. Needs Python 3 and rsvg-convert.
.PHONY: screenshots
screenshots:
	d=$$(mktemp -d) && SITREP_README_DIR=$$d go test ./internal/cli -run TestWriteReadmeShots -count=1 && python3 docs/theme-shots/readme_shots.py $$d docs/img

.PHONY: build
build:
	go build -o sitrep ./cmd/sitrep

# Builds the program beside the installed one and lets it install itself, as /sitrep:setup does.
.PHONY: install
install:
	mkdir -p "$$HOME/.claude/sitrep"
	go build -o "$$HOME/.claude/sitrep/sitrep.new" -mod=readonly -buildvcs=false ./cmd/sitrep
	"$$HOME/.claude/sitrep/sitrep.new" setup
