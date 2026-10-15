.PHONY: dev-server dev-web build test check docker

# Run the Go API on :8080 (reads ../.env if present).
dev-server:
	cd server && set -a && ([ -f ../.env ] && . ../.env || true) && set +a && STATIC_DIR=../web/build go run ./cmd/blastingroom

# Run the Svelte dev server on :5173, proxying /api to :8080.
dev-web:
	cd web && npm run dev

build:
	cd web && npm run build
	cd server && go build -o bin/blastingroom ./cmd/blastingroom

test:
	cd server && go vet ./... && go test ./...

check: test
	cd web && npm run check

docker:
	docker build -t blasting-room .
