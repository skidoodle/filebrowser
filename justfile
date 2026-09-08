set shell := ["pwsh", "-c"]

version := `git describe --tags --always --dirty`
commit := `git rev-parse --short HEAD`
exe := if os() == "windows" { ".exe" } else { "" }
# Prefer the go-installed golangci-lint (matches the local toolchain);
# PATH may carry an older binary built with a previous Go version.
golangci := if os() == "windows" { env("GOPATH", "") + "\\bin\\golangci-lint.exe" } else { env("GOPATH", "") + "/bin/golangci-lint" }

# default: build everything
default: build

# build: build the frontend bundle and the backend binary into bin/
build: frontend backend

# backend: compile the Go binary (embeds frontend/dist)
backend:
    go build -trimpath -ldflags "-s -w -X main.version={{ version }} -X main.commit={{ commit }}" -o bin/filebrowser{{ exe }} .

# frontend: install deps and build the SPA into frontend/dist
frontend:
    bun install --frozen-lockfile --cwd frontend
    bun run --cwd frontend build

# dev: run the Go dev server (live reload via air) and the Vite dev server together in the same terminal
dev:
    bun x concurrently -k -n "air,vite" -c "magenta,cyan" "air" "bun run --cwd frontend dev"

# dev-backend: run the API in dev mode with live reload via air
dev-backend:
    air

# dev-frontend: run the Vite dev server (proxies /api to 127.0.0.1:8080)
dev-frontend:
    bun run --cwd frontend dev

# test: run all tests with the race detector
test:
    go test -race ./...

# lint: run golangci-lint
lint:
    {{ golangci }} run

# lint-fix: run golangci-lint with auto-fixes
lint-fix:
    {{ golangci }} run --fix

# fmt: format + vet Go code
fmt:
    gofmt -s -w .
    go vet ./...

# clean: remove build artifacts
clean:
    Remove-Item -Recurse -Force -ErrorAction SilentlyContinue bin, coverage.out, coverage.html, tmp

# release: tag HEAD as vX.Y.Z and push the tag (triggers the Release workflow)
release ver:
    git tag "v{{ trim_start_matches(ver, "v") }}"
    git push origin "v{{ trim_start_matches(ver, "v") }}"

# docker: run the dev environment
docker:
    docker compose -f compose.dev.yaml up --build
