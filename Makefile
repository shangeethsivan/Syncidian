.PHONY: build test run docker plugin plugin-install

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
