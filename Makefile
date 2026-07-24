# go_dev_tool_version_manager 開発用タスク。
# 運用決定は docs/reports/W00-planning.md（W00-03/W00-04）を参照。
# 生の `go` command でも同じ検証を再現できる（Windows含む）。

GO ?= go
ARTIFACTS := .artifacts
COVER_DIR := $(ARTIFACTS)/coverage
GOLANGCI_VERSION := v2.5.0

.PHONY: all fmt fmt-check vet lint test test-race cover build tidy tools clean help

all: fmt-check vet test ## 既定: 整形確認・vet・test

fmt: ## gofmtで整形
	$(GO) fmt ./...

fmt-check: ## 未整形ファイルがあれば失敗
	@files=$$(gofmt -l .); if [ -n "$$files" ]; then echo "gofmt required for:"; echo "$$files"; exit 1; fi

vet: ## go vet
	$(GO) vet ./...

lint: ## golangci-lint（対象Go版以上でbuildした版が必要。CI相当）
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./... || echo "note: ローカルgolangci-lintが対象Go版未満のbuildだと失敗する。CIで実行する"; \
	else \
		echo "golangci-lint 未導入。'make tools' で導入するか、CIで実行する"; \
	fi

test: ## unit test
	$(GO) test ./...

test-race: ## race detector付きtest
	$(GO) test -race ./...

cover: ## coverage取得（.artifacts/coverage/）
	@mkdir -p $(COVER_DIR)
	$(GO) test -covermode=atomic -coverprofile=$(COVER_DIR)/coverage.out ./...
	@$(GO) tool cover -func=$(COVER_DIR)/coverage.out | tail -1

build: ## 開発build（現在host、cmd/gdtvm 実装後に有効）
	@mkdir -p $(ARTIFACTS)
	CGO_ENABLED=0 $(GO) build -o $(ARTIFACTS)/gdtvm ./cmd/gdtvm

tidy: ## go mod tidy
	$(GO) mod tidy

tools: ## 開発toolを対象Go版でbuild導入（golangci-lint等）
	$(GO) install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_VERSION)
	$(GO) install golang.org/x/vuln/cmd/govulncheck@latest

clean: ## artifacts削除
	rm -rf $(ARTIFACTS)

help: ## このヘルプ
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-12s\033[0m %s\n",$$1,$$2}'
