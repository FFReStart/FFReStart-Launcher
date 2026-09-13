FROM node:22.12.0-bookworm-slim AS frontend
WORKDIR /src/frontend
RUN npm install --global pnpm@9.15.1
COPY frontend/package.json frontend/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY frontend/ ./
COPY GameLauncher/images/*.png /src/GameLauncher/images/
RUN pnpm run build

FROM golang:1.27.1-bookworm AS builder
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update \
    && apt-get install --yes --no-install-recommends build-essential libgtk-3-dev libwebkit2gtk-4.1-dev pkg-config \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . ./
COPY --from=frontend /src/frontend/dist ./frontend/dist
RUN go install github.com/wailsapp/wails/v2/cmd/wails@v2.15.0 \
    && wails build -platform linux/amd64 -tags webkit2_41 -clean -s

FROM scratch AS export
COPY --from=builder /src/build/bin/ffrestart-launcher /
