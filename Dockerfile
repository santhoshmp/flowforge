# syntax=docker/dockerfile:1

# FlowForge single-binary image — multi-stage:
#   1. ui  — Studio UI (React/Vite) + @flowforge/dsl, ALWAYS built on the
#            native BUILD platform (the output is platform-independent)
#   2. go  — static per-target cross-compile (CGO_ENABLED=0; sqlite/wazero/
#            starlark are pure Go), also native — no QEMU emulation anywhere
#   3. run — minimal alpine, non-root user, /data volume for SQLite
#
# Multi-arch (linux/amd64 + linux/arm64) therefore only varies the final
# base image — the whole build stays fast instead of emulating node/go.

# ---- 1. UI -------------------------------------------------------------------
FROM --platform=$BUILDPLATFORM node:22-alpine AS ui
WORKDIR /src
COPY dsl/package.json dsl/package-lock.json ./dsl/
RUN npm --prefix dsl ci --no-fund --no-audit
COPY dsl ./dsl
RUN npm --prefix dsl run build
COPY app/package.json app/package-lock.json ./app/
RUN npm --prefix app ci --no-fund --no-audit
COPY app ./app
RUN npm --prefix app run build

# ---- 2. Go binary (cross-compiled per target platform) ------------------------
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS go
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
ENV CGO_ENABLED=0
COPY server-go/go.mod server-go/go.sum ./server-go/
RUN cd server-go && go mod download
COPY server-go ./server-go
COPY --from=ui /src/app/dist /ui-dist
RUN cd server-go \
 && rm -rf ui/dist && mkdir ui/dist && cp -r /ui-dist/. ui/dist/ \
 && GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags "-s -w" -o /out/flowforge ./cmd/flowforge

# ---- 3. Runtime ---------------------------------------------------------------
FROM alpine:3.20
# Create /data with the right ownership BEFORE switching users: Docker
# initializes volumes from the image directory's ownership, and the non-root
# user must be able to create the SQLite database there.
RUN addgroup -S flowforge && adduser -S flowforge -G flowforge \
 && mkdir -p /data && chown flowforge:flowforge /data
COPY --from=go /out/flowforge /usr/local/bin/flowforge
ENV PORT=8080 \
    DB_PATH=/data/flowforge.db
USER flowforge
WORKDIR /data
EXPOSE 8080
VOLUME ["/data"]
ENTRYPOINT ["flowforge"]
CMD ["serve"]
