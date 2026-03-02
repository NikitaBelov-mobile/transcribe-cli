SHELL := /bin/bash

GO ?= go
GOCACHE ?= $(CURDIR)/.gocache
DIST_DIR ?= $(CURDIR)/dist/local
DESKTOP_DIST_DIR ?= $(CURDIR)/dist/desktop
REMOTE ?= origin
BRANCH ?= main

.DEFAULT_GOAL := help

.PHONY: help clean check fmt test vet build-cli build-cli-current build-cli-all build-desktop-win build-desktop-winforms build-desktop-winui \
	git-status git-commit git-push git-tag git-push-tag release-tag

help: ## Show available tasks
	@grep -E '^[a-zA-Z0-9._-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-24s\033[0m %s\n", $$1, $$2}'

clean: ## Remove local build artifacts
	rm -rf "$(DIST_DIR)" "$(DESKTOP_DIST_DIR)" "$(CURDIR)/.gocache"

fmt: ## Run gofmt for all Go files
	@files="$$(find . -name '*.go' -type f)"; \
	if [ -n "$$files" ]; then gofmt -w $$files; fi

test: ## Run go tests
	GOCACHE="$(GOCACHE)" $(GO) test ./...

vet: ## Run go vet
	GOCACHE="$(GOCACHE)" $(GO) vet ./...

check: test vet ## Run main checks (test + vet)

build-cli-current: ## Build CLI for current platform
	mkdir -p "$(DIST_DIR)"
	GOCACHE="$(GOCACHE)" $(GO) build -ldflags "-s -w" -o "$(DIST_DIR)/transcribe" ./cmd/transcribe-cli

build-cli: build-cli-current ## Alias for build-cli-current

build-cli-all: ## Build CLI for darwin/windows amd64+arm64
	mkdir -p "$(DIST_DIR)"
	@set -euo pipefail; \
	targets=(darwin/amd64 darwin/arm64 windows/amd64 windows/arm64); \
	for target in "$${targets[@]}"; do \
		os="$${target%/*}"; arch="$${target#*/}"; \
		out="$(DIST_DIR)/transcribe_$${os}_$${arch}"; \
		if [ "$$os" = "windows" ]; then out="$$out.exe"; fi; \
		echo "Building $$os/$$arch -> $$out"; \
		GOOS="$$os" GOARCH="$$arch" CGO_ENABLED=0 GOCACHE="$(GOCACHE)" $(GO) build -ldflags "-s -w" -o "$$out" ./cmd/transcribe-cli; \
	done

build-desktop-win: build-desktop-winui ## Build native Windows desktop bundle (WinUI + transcribe.exe)

build-desktop-winforms: ## Build legacy WinForms desktop bundle (TranscribeDesktop.exe + transcribe.exe)
	@command -v dotnet >/dev/null 2>&1 || { echo "dotnet is required for this task"; exit 1; }
	mkdir -p "$(DESKTOP_DIST_DIR)/bundle"
	GOCACHE="$(GOCACHE)" GOOS=windows GOARCH=amd64 CGO_ENABLED=0 $(GO) build -ldflags "-s -w" -o "$(DESKTOP_DIST_DIR)/bundle/transcribe.exe" ./cmd/transcribe-cli
	dotnet publish desktop-windows/TranscribeDesktop.Windows.csproj \
		-c Release \
		-r win-x64 \
		--self-contained true \
		-p:PublishSingleFile=true \
		-p:IncludeNativeLibrariesForSelfExtract=true \
		-o "$(DESKTOP_DIST_DIR)/publish"
	cp "$(DESKTOP_DIST_DIR)/publish/TranscribeDesktop.exe" "$(DESKTOP_DIST_DIR)/bundle/TranscribeDesktop.exe"
	@echo "Desktop bundle: $(DESKTOP_DIST_DIR)/bundle"

build-desktop-winui: ## Build WinUI desktop bundle (TranscribeDesktop.WinUI.exe + transcribe.exe)
	@command -v dotnet >/dev/null 2>&1 || { echo "dotnet is required for this task"; exit 1; }
	mkdir -p "$(DESKTOP_DIST_DIR)/bundle-winui"
	GOCACHE="$(GOCACHE)" GOOS=windows GOARCH=amd64 CGO_ENABLED=0 $(GO) build -ldflags "-s -w" -o "$(DESKTOP_DIST_DIR)/bundle-winui/transcribe.exe" ./cmd/transcribe-cli
	dotnet publish desktop-windows-winui/TranscribeDesktop.WinUI.csproj \
		-c Release \
		-r win-x64 \
		--self-contained true \
		-p:PublishSingleFile=true \
		-p:IncludeNativeLibrariesForSelfExtract=true \
		-o "$(DESKTOP_DIST_DIR)/publish-winui"
	cp "$(DESKTOP_DIST_DIR)/publish-winui/TranscribeDesktop.WinUI.exe" "$(DESKTOP_DIST_DIR)/bundle-winui/TranscribeDesktop.exe"
	@echo "WinUI desktop bundle: $(DESKTOP_DIST_DIR)/bundle-winui"

git-status: ## Show short git status
	git status --short --branch

git-commit: ## Stage all changes and commit. Usage: make git-commit MSG="your message"
	@test -n "$(MSG)" || (echo 'Usage: make git-commit MSG="your message"' && exit 1)
	git add -A
	git commit -m "$(MSG)"

git-push: ## Push current branch. Usage: make git-push [REMOTE=origin] [BRANCH=main]
	git push "$(REMOTE)" "$(BRANCH)"

git-tag: ## Create local git tag. Usage: make git-tag TAG=v0.3.11
	@test -n "$(TAG)" || (echo 'Usage: make git-tag TAG=v0.3.11' && exit 1)
	git tag "$(TAG)"

git-push-tag: ## Push tag to remote. Usage: make git-push-tag TAG=v0.3.11 [REMOTE=origin]
	@test -n "$(TAG)" || (echo 'Usage: make git-push-tag TAG=v0.3.11 [REMOTE=origin]' && exit 1)
	git push "$(REMOTE)" "$(TAG)"

release-tag: ## Push branch and tag. Usage: make release-tag TAG=v0.3.11 [REMOTE=origin] [BRANCH=main]
	@test -n "$(TAG)" || (echo 'Usage: make release-tag TAG=v0.3.11' && exit 1)
	git push "$(REMOTE)" "$(BRANCH)"
	git tag "$(TAG)"
	git push "$(REMOTE)" "$(TAG)"
