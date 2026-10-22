# syntax=docker/dockerfile:1
# LeftOff server image (SPEC §3, Phase 4). Multi-arch: the UI and the
# Go binary are built on the build platform and cross-compiled for the
# target, so `docker buildx build --platform linux/amd64,linux/arm64` works
# without emulation. No browser inside: use FlareSolverr for Cloudflare.

ARG GO_VERSION=1.26
ARG NODE_VERSION=24

FROM --platform=$BUILDPLATFORM node:${NODE_VERSION}-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-alpine AS build
ARG TARGETOS=linux
ARG TARGETARCH=amd64
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath \
      -ldflags "-s -w -X main.version=$VERSION" -o /out/leftoff ./cmd/server \
 && mkdir -p /out/data

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/leftoff /usr/local/bin/leftoff
COPY --from=build --chown=nonroot:nonroot /out/data /data
ENV DATA_DIR=/data PORT=8080
VOLUME ["/data"]
EXPOSE 8080
USER nonroot
HEALTHCHECK --interval=1m --timeout=5s --start-period=10s CMD ["/usr/local/bin/leftoff", "health"]
ENTRYPOINT ["/usr/local/bin/leftoff"]
CMD ["serve"]
