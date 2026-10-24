.PHONY: dev-server dev-web build test check docker probe

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

# Run the Day-1 data probes against live Qloo (reads ../.env). Prints to the
# terminal only; never save the output in the repo (Qloo data can't be committed).
# Pass flags with ARGS, e.g. make probe ARGS="-only 1,6 -artist Wednesday".
probe:
	cd server && set -a && ([ -f ../.env ] && . ../.env || true) && set +a && go run ./cmd/probe $(ARGS)

docker:
	docker build -t blasting-room .
