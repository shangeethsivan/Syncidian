.PHONY: build test run docker plugin plugin-install plugin-manifest

build:
	go build -o bin/syncidian ./cmd/syncidian

test:
	go test ./...

run:
	go run ./cmd/syncidian serve

docker:
	docker build -t syncidian .

plugin:
	cd plugin && npm install && npm run build

plugin-install:
	@test -n "$(VAULT)" || (echo "Usage: make plugin-install VAULT=/path/to/vault" && exit 1)
	./scripts/install-plugin.sh "$(VAULT)"

plugin-manifest:
	./scripts/sync-plugin-manifest.sh
