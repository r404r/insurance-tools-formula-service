.PHONY: all build run dev test clean backend-build backend-run backend-test frontend-build frontend-dev frontend-install migrate

# Default target
all: build

# ─── Backend ───────────────────────────────────────────────
backend-build:
	cd backend && go build -o bin/server ./cmd/server

backend-run: backend-build
	cd backend && ./bin/server

backend-test:
	cd backend && go test ./... -v

backend-vet:
	cd backend && go vet ./...

# ─── Frontend ──────────────────────────────────────────────
frontend-install:
	cd frontend && npm install

frontend-dev:
	cd frontend && npm run dev

frontend-build:
	cd frontend && npm run build

frontend-lint:
	cd frontend && npm run lint

# ─── Combined ──────────────────────────────────────────────
build: backend-build frontend-build

dev:
	@echo "Starting backend and frontend in parallel..."
	@trap 'kill 0' INT TERM; \
	(cd backend && go run ./cmd/server) & \
	(cd frontend && npm run dev) & \
	wait

test: backend-test

clean:
	rm -rf backend/bin
	rm -rf frontend/dist
	rm -f backend/*.db backend/*.db-*

# ─── Database ──────────────────────────────────────────────
migrate:
	cd backend && go run ./cmd/server -migrate

# ─── Docker ────────────────────────────────────────────────
docker-build:
	docker compose build

docker-up:
	docker compose up -d

docker-down:
	docker compose down
