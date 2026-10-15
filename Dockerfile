# syntax=docker/dockerfile:1
FROM node:24-alpine AS frontend
WORKDIR /src/web
RUN npm install --global pnpm@11.28.5
COPY web/package.json web/pnpm-lock.yaml web/pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile
COPY web/ ./
RUN pnpm run build

FROM golang:1.26-alpine AS build
RUN apk add --no-cache build-base
WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
COPY --from=frontend /src/web/dist ./internal/webui/dist
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown
ENV CGO_ENABLED=1
RUN release_flags="-s -w -linkmode external -extldflags '-static' -X campfire/internal/buildinfo.Version=${VERSION} -X campfire/internal/buildinfo.Commit=${COMMIT} -X campfire/internal/buildinfo.Date=${BUILD_DATE}" \
    && go build -tags production -buildvcs=false -trimpath -ldflags "$release_flags" -o /out/campfire ./cmd/campfire \
    && go build -buildvcs=false -trimpath -ldflags "$release_flags" -o /out/spark ./cmd/spark \
    && go test -tags production ./internal/webui

# Export statically linked Linux binaries without a dependency on host glibc.
FROM scratch AS binaries
COPY --from=build /out/ /

FROM alpine:3.23 AS runtime
RUN apk add --no-cache ca-certificates \
    && addgroup -g 10001 campfire \
    && adduser -D -H -u 10001 -G campfire campfire \
    && mkdir -p /var/lib/campfire \
    && chown 10001:10001 /var/lib/campfire
COPY --from=build /out/campfire /usr/local/bin/campfire
USER 10001:10001
WORKDIR /var/lib/campfire
ENV CAMPFIRE_DATABASE=/var/lib/campfire/panel.db \
    CAMPFIRE_LISTEN=0.0.0.0:8080
EXPOSE 8080 8443
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
    CMD wget -q -O /dev/null http://127.0.0.1:8080/healthz || exit 1
ENTRYPOINT ["/usr/local/bin/campfire"]
