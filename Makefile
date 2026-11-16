BUILD_DIR := build
TARGETS := darwin/amd64 darwin/arm64 freebsd/amd64 \
	linux/386 linux/amd64 linux/arm5 linux/arm6 linux/arm7 \
	linux/arm64 linux/loong64 linux/mips linux/mipsle \
	windows/386 windows/amd64 windows/arm64

.PHONY: all build check-go clean

all: build

build: check-go
	@set -e; \
	if command -v sha1sum >/dev/null 2>&1; then \
		sum_tool=sha1sum; \
	elif command -v shasum >/dev/null 2>&1; then \
		sum_tool=shasum; \
	else \
		echo "Error: Neither 'sha1sum' nor 'shasum' tool found."; exit 1; \
	fi; \
	if command -v upx >/dev/null 2>&1; then \
		use_upx=true; echo "Info: UPX found. Binaries will be compressed."; \
	else \
		use_upx=false; echo "Info: UPX not found. Skipping compression step."; \
	fi; \
	if tar --no-xattrs -cf /dev/null --files-from /dev/null >/dev/null 2>&1; then \
		tar_no_xattrs=--no-xattrs; \
	else \
		tar_no_xattrs=; echo "Warning: tar does not support --no-xattrs. Archives may contain xattr metadata."; \
	fi; \
	version="$$(date -u +%Y%m%d)"; \
	mkdir -p "$(BUILD_DIR)"; \
	build_dir="$$(cd "$(BUILD_DIR)" && pwd)"; \
	for target in $(TARGETS); do \
		os="$${target%/*}"; suffix="$${target#*/}"; \
		arch="$$suffix"; goarm=; gomips=; ext=; \
		case "$$suffix" in \
			arm5|arm6|arm7) arch=arm; goarm="$${suffix#arm}" ;; \
			mips|mipsle) gomips=softfloat ;; \
		esac; \
		if [ "$$os" = windows ]; then ext=.exe; fi; \
		client="client_$${os}_$${suffix}$${ext}"; \
		server="server_$${os}_$${suffix}$${ext}"; \
		echo "--- Building $$os/$$suffix ---"; \
		export GO111MODULE=on CGO_ENABLED=0 GOOS="$$os" GOARCH="$$arch" GOARM="$$goarm" GOMIPS="$$gomips"; \
		go build -mod=vendor -ldflags "-X main.VERSION=$$version -s -w" -o "$$build_dir/$$client" ./client; \
		go build -mod=vendor -ldflags "-X main.VERSION=$$version -s -w" -o "$$build_dir/$$server" ./server; \
		if $$use_upx; then \
			upx "$$build_dir/$$client" "$$build_dir/$$server" || echo "Warning: UPX compression failed."; \
		fi; \
		archive="kcptun-$$os-$$suffix-$$version.tar.gz"; \
		echo "--- Packaging $$archive ---"; \
		(cd "$$build_dir" && COPYFILE_DISABLE=1 tar $$tar_no_xattrs -czf "$$archive" "$$client" "$$server"); \
		rm -f "$$build_dir/$$client" "$$build_dir/$$server"; \
	done; \
	echo "--- Generating SHA1 Checksums ---"; \
	(cd "$$build_dir" && $$sum_tool *.tar.gz > SHA1SUMS && cat SHA1SUMS); \
	echo "--- Build Complete ---"; \
	echo "All release packages are located in $$build_dir/"

check-go:
	@command -v go >/dev/null 2>&1 || { \
		echo "Error: Go toolchain not found. Install Go 1.26.0 or later and ensure 'go' is in PATH."; \
		exit 1; \
	}
	@version="$$(go env GOVERSION)" || { echo "Error: Unable to determine Go version."; exit 1; }; \
	version="$${version#go}"; \
	case "$$version" in \
		*.*) ;; \
		*) echo "Error: Unable to determine Go version: $$version."; exit 1 ;; \
	esac; \
	major="$${version%%.*}"; \
	remainder="$${version#*.}"; \
	minor="$${remainder%%[!0-9]*}"; \
	case "$$major:$$minor" in \
		*[!0-9:]*|:*|*:) echo "Error: Unable to determine Go version: $$version."; exit 1 ;; \
	esac; \
	if [ "$$major" -lt 1 ] || { [ "$$major" -eq 1 ] && [ "$$minor" -lt 26 ]; } || \
		{ [ "$$major" -eq 1 ] && [ "$$minor" -eq 26 ] && \
			case "$$remainder" in 26beta*|26rc*) true ;; *) false ;; esac; }; then \
		echo "Error: Go 1.26.0 or later is required; found Go $$version."; \
		exit 1; \
	fi; \
	go version

clean:
	rm -rf "$(BUILD_DIR)"
