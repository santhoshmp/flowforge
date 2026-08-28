# syntax=docker/dockerfile:1

# FlowForge single-binary image — multi-stage:
#   1. ui  — build the Studio UI (React/Vite) and the @flowforge/dsl package
#            it consumes via file:../dsl
#   2. go  — static build (CGO_ENABLED=0; sqlite/wazero/starlark are pure Go)
#            with the UI embedded via embed.FS
#   3. run — minimal alpine with a non-root user and a /data volume for SQLite

# ---- 1. UI -------------------------------------------------------------------
FROM node:22-alpine AS ui
WORKDIR /src
COPY dsl/package.json dsl/package-lock.json ./dsl/
RUN npm --prefix dsl ci --no-fund --no-audit
COPY dsl ./dsl
RUN npm --prefix dsl run build
COPY app/package.json app/package-lock.json ./app/
RUN npm --prefix app ci --no-fund --no-audit
COPY app ./app
RUN npm --prefix app run build

# ---- 2. Go binary -------------------------------------------------------------
FROM golang:1.26-alpine AS go
WORKDIR /src
COPY server-go/go.mod server-go/go.sum ./server-go/
RUN cd server-go && go mod download
COPY server-go ./server-go
# Replace the placeholder embed with the built UI.
COPY --from=ui /src/app/dist ./server-go/ui/dist
RUN cd server-go && \
    CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /out/flowforge ./cmd/flowforge

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
