# syntax=docker/dockerfile:1

# 1. Build the Svelte single-page app.
FROM node:22-alpine AS web
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# 2. Build the Go server.
FROM golang:1.24-alpine AS server
WORKDIR /src
COPY server/go.mod server/go.sum ./
RUN go mod download
COPY server/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/blastingroom ./cmd/blastingroom

# 3. Small runtime image: one binary plus the static web build.
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=server /out/blastingroom /app/blastingroom
COPY --from=web /web/build /app/public
ENV PORT=8080 STATIC_DIR=/app/public
EXPOSE 8080
USER nonroot
# Distroless has no curl or wget, so the binary checks itself.
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 CMD ["/app/blastingroom", "-healthcheck"]
ENTRYPOINT ["/app/blastingroom"]
