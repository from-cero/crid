MODULE = github.com/from-cero/crid

format-tools:
	go install mvdan.cc/gofumpt@latest
	go install golang.org/x/tools/cmd/goimports@latest
	go install github.com/daixiang0/gci@latest
	go install github.com/segmentio/golines@latest

format:
	go mod tidy
	goimports -w .
	gci write --custom-order -s standard -s default -s "prefix($(MODULE))" -s blank \
		--no-lex-order --skip-generated --skip-vendor .
	golines -w -m 120 .
	gofumpt -l -w -extra .

lint-tools:
	curl -sSfL https://golangci-lint.run/install.sh | sh -s -- -b $$(go env GOPATH)/bin v2.12.1

lint:
	golangci-lint run ./...

test:
	go test -race -cover ./...

test-coverage:
	go test -race -coverprofile=coverage.out ./... && go tool cover -html=coverage.out

precommit: format lint test

release:
	@test -n "$(VERSION)" || (echo "VERSION required, e.g. make release VERSION=v0.1.0"; exit 1)
	git tag $(VERSION)
	git push origin $(VERSION)
	cd registry/postgres && go mod edit -require=$(MODULE)@$(VERSION) && go mod tidy
	git add registry/postgres/go.mod registry/postgres/go.sum
	git commit -m "chore: bump $(MODULE) to $(VERSION) in postgres registry"
	git push
	git tag registry/postgres/$(VERSION)
	git push origin registry/postgres/$(VERSION)