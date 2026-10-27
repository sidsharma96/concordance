.PHONY: verify check fmt-check test race build artifacts docs hooks-install

verify:
	@test -n "$${CONCORDANCE_TEST_DATABASE_URL:-}" || { echo "Set CONCORDANCE_TEST_DATABASE_URL to a dedicated test database before make verify." >&2; exit 1; }
	$(MAKE) check
	go test -count=1 -race ./...
	$(MAKE) build

check: fmt-check
	go vet ./...
	go test ./...
	go run ./tools/checkrepo

fmt-check:
	@test -z "$$(gofmt -l cmd internal tools)" || { gofmt -l cmd internal tools; exit 1; }

test:
	go test ./...

race:
	go test -race ./...

build:
	go build -o bin/concordance ./cmd/concordance

artifacts:
	go run ./tools/checkrepo

docs:
	go run ./tools/checkdocs
	mdbook build docs
	go run ./tools/checkdocs --site docs/book

hooks-install:
	@current=$$(git config --get core.hooksPath || true); \
	if [ -n "$$current" ] && [ "$$current" != ".githooks" ]; then \
	  echo "Refusing to replace existing core.hooksPath: $$current" >&2; exit 1; \
	fi; \
	git config --local core.hooksPath .githooks
