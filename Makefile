# Local secrets and host details live in .env (gitignored). Format:
#   DEPLOY_HOST=user@host
#   DEPLOY_KEY=/path/to/key
#   DEPLOY_ARCH=linux-arm64
-include .env

BINARY  := claude-sync
PKG     := github.com/fpirim/claude-sync
VERSION ?= $(shell git -C $(CURDIR) describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X '$(PKG)/cmd.Version=$(VERSION)'
GOFLAGS := -trimpath -ldflags '$(LDFLAGS)'

.PHONY: build dist clean install test fmt vet deploy setup help

# Deploy target reads its host + key from env vars. Set them in your shell or
# in a local .env file (which is gitignored) — keep secrets out of source.
#   DEPLOY_HOST  user@host         (e.g. ubuntu@1.2.3.4 or alias from ~/.ssh/config)
#   DEPLOY_KEY   /path/to/key      (optional; omit if SSH agent or config handles it)
#   DEPLOY_ARCH  linux-arm64       (linux-arm64 | linux-amd64)
DEPLOY_HOST ?=
DEPLOY_KEY  ?=
DEPLOY_ARCH ?= linux-arm64

help:
	@echo "claude-sync make targets"
	@echo "  setup        full bootstrap: deps + binary on local AND remote (reads .env)"
	@echo "  install      build and place binary in \$$HOME/bin"
	@echo "  deploy       cross-build linux binary and push to remote"
	@echo "  build        build local binary into ./$(BINARY)"
	@echo "  dist         cross-compile darwin-arm64 + linux-{arm64,amd64}"
	@echo "  test fmt vet standard Go workflow"
	@echo "  clean        remove dist/ and ./$(BINARY)"
	@echo ""
	@echo "Remote settings come from .env (gitignored). See .env.example."

# All-in-one bootstrap. Idempotent — safe to re-run after editing .env or
# pulling new code. Skips remote steps if DEPLOY_HOST is unset.
setup:
	@echo ">> [1/4] installing local deps (syncthing)"
	./scripts/install-deps.sh
	@echo ">> [2/4] installing local binary"
	@$(MAKE) --no-print-directory install
	@if [ -z "$(DEPLOY_HOST)" ]; then \
		echo ">> remote steps skipped: DEPLOY_HOST not set in .env"; \
		exit 0; \
	fi
	@echo ">> [3/4] installing remote deps on $(DEPLOY_HOST)"
	$(if $(DEPLOY_KEY),scp -i "$(DEPLOY_KEY)",scp) scripts/install-deps.sh $(DEPLOY_HOST):/tmp/claude-sync-install-deps.sh
	$(if $(DEPLOY_KEY),ssh -i "$(DEPLOY_KEY)",ssh) $(DEPLOY_HOST) 'chmod +x /tmp/claude-sync-install-deps.sh && /tmp/claude-sync-install-deps.sh && rm /tmp/claude-sync-install-deps.sh'
	@echo ">> [4/5] deploying binary to $(DEPLOY_HOST)"
	@$(MAKE) --no-print-directory deploy
	@if [ -z "$(GUI_USER)" ] || [ -z "$(GUI_PASSWORD)" ]; then \
	    echo ">> [5/5] skipping syncthing pairing: GUI_USER/GUI_PASSWORD not set in .env"; \
	    echo ">> setup complete (manual Syncthing pairing required)"; \
	else \
	    echo ">> [5/5] pairing Syncthing instances + sharing folder"; \
	    GUI_USER='$(GUI_USER)' GUI_PASSWORD='$(GUI_PASSWORD)' \
	    FOLDER_ID='$(or $(FOLDER_ID),claude-home)' \
	    FOLDER_PATH='$(or $(FOLDER_PATH),~/.claude)' \
	    DEPLOY_HOST='$(DEPLOY_HOST)' DEPLOY_KEY='$(DEPLOY_KEY)' \
	        ./scripts/sync-setup.sh && \
	    echo ">> setup complete"; \
	fi

build:
	go build $(GOFLAGS) -o $(BINARY) .

# Cross-compile static binaries for the three targets we care about.
# CGO_ENABLED=0 makes the result self-contained (no glibc surprises on
# minimal Linux images).
dist: clean
	@mkdir -p dist
	@echo ">> darwin/arm64"
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build $(GOFLAGS) -o dist/$(BINARY)-darwin-arm64 .
	@echo ">> linux/arm64"
	CGO_ENABLED=0 GOOS=linux  GOARCH=arm64 go build $(GOFLAGS) -o dist/$(BINARY)-linux-arm64  .
	@echo ">> linux/amd64"
	CGO_ENABLED=0 GOOS=linux  GOARCH=amd64 go build $(GOFLAGS) -o dist/$(BINARY)-linux-amd64  .
	@ls -lh dist/

clean:
	rm -rf dist $(BINARY)

install: build
	@mkdir -p $(HOME)/bin
	install -m 0755 $(BINARY) $(HOME)/bin/$(BINARY)
	@echo ">> installed to $(HOME)/bin/$(BINARY)"
	@case ":$$PATH:" in *":$(HOME)/bin:"*) ;; *) echo "!! note: $(HOME)/bin is not in PATH; add to your shell rc";; esac

test:
	go test ./...

fmt:
	gofmt -s -w .

vet:
	go vet ./...

# One-shot push to a remote host. Reads DEPLOY_HOST, DEPLOY_KEY, DEPLOY_ARCH
# from the environment or .env (sourced via -include). Examples:
#   make deploy DEPLOY_HOST=ubuntu@1.2.3.4 DEPLOY_KEY=~/keys/foo.pem
#   make deploy DEPLOY_HOST=myhost           # SSH alias, no key needed
deploy: dist
	@if [ -z "$(DEPLOY_HOST)" ]; then \
		echo "DEPLOY_HOST not set. Run: make deploy DEPLOY_HOST=user@host [DEPLOY_KEY=path] [DEPLOY_ARCH=linux-arm64]"; \
		exit 1; \
	fi
	$(if $(DEPLOY_KEY),scp -i "$(DEPLOY_KEY)",scp) dist/$(BINARY)-$(DEPLOY_ARCH) $(DEPLOY_HOST):/tmp/claude-sync
	$(if $(DEPLOY_KEY),ssh -i "$(DEPLOY_KEY)",ssh) $(DEPLOY_HOST) 'mkdir -p ~/bin && mv /tmp/claude-sync ~/bin/claude-sync && chmod +x ~/bin/claude-sync && ~/bin/claude-sync --version'
