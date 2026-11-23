# Contributing

sitrep is one Go program. `make build` builds `./sitrep`, and `make test` is the one check, which CI also runs on Linux and macOS: `gofmt`, `go mod tidy -diff`, `go vet` and `go test -race`.

The tests use a throwaway `$HOME` and a stub `claude`, so they never touch real settings or the API. The scenario tests run the built program through commands and hook events and compare with golden files (`go test ./internal/cli -update-golden` rewrites them). They began as comparisons with the last shell version, the git tag `bash-final`, which nothing uses now. `docs/GO-PORT.md` describes the port from shell to Go.

The rules the code keeps (untrusted text, owner-only files, hooks that never fail, and the rest) are in [AGENTS.md](AGENTS.md): read them before a change. Add a test for every behavior or bug fix. The pictures of the README are made by `make screenshots` (see [docs/THEMES.md](docs/THEMES.md)).
