set dotenv-load := false

check:
    pnpm --dir frontend install --frozen-lockfile
    pnpm --dir frontend run typecheck
    pnpm --dir frontend run build
    go vet ./...
    go test ./...
    golangci-lint run

build-windows:
    wails build -platform windows/amd64 -clean

build-linux:
    docker build --file build/linux.Dockerfile --output type=local,dest=build/linux-out .

build-release platform="windows/amd64":
    go run ./cmd/releasebuild {{platform}}
