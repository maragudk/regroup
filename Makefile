-include .env

APP_NAME ?= app
DATABASE_PATH ?= app.db

.PHONY: benchmark
benchmark:
	go test -tags sqlite_fts5,sqlite_math_functions -bench . ./...

.PHONY: build-docker
build-docker:
	docker build --platform linux/arm64 -t $(APP_NAME) .

.PHONY: clean-all
clean-all: test-down
	rm -rf .versitygw
	rm -f $(DATABASE_PATH) $(DATABASE_PATH)-wal $(DATABASE_PATH)-shm

.PHONY: cover
cover:
	go tool cover -html cover.out

.PHONY: deps
deps:
	curl -Lf -o public/scripts/datastar.js https://cdn.jsdelivr.net/gh/starfederation/datastar@1.0.3/bundles/datastar.js

# Fetches the fonts the app serves from their own repositories, at fixed commits, with their licences.
# Source Sans 3 ships as WOFF2. Fraunces ships its variable font as TrueType only, so it is put in a WOFF2
# container here, which changes nothing in the font itself, with fonttools run by uv. A test holds the
# checksums of the four files, so it has to follow when one of them changes.
.PHONY: fonts
fonts: FRAUNCES_TTF := $(shell mktemp -t Fraunces.XXXXXX)
fonts:
	curl -Lf -o public/styles/fonts/SourceSans3-Regular.woff2 https://raw.githubusercontent.com/adobe-fonts/source-sans/87b37a2daaed80fcb8e8ccb0085c4d72ddade12e/WOFF2/TTF/SourceSans3-Regular.ttf.woff2
	curl -Lf -o public/styles/fonts/SourceSans3-LICENSE.md https://raw.githubusercontent.com/adobe-fonts/source-sans/87b37a2daaed80fcb8e8ccb0085c4d72ddade12e/LICENSE.md
	curl -Lf -o public/styles/fonts/Fraunces-OFL.txt https://raw.githubusercontent.com/undercasetype/Fraunces/7ccdec31c6028118dce3e47fe864e3744460371d/OFL.txt
	curl -Lf -o $(FRAUNCES_TTF) 'https://raw.githubusercontent.com/undercasetype/Fraunces/7ccdec31c6028118dce3e47fe864e3744460371d/fonts/variable/Fraunces%5BSOFT%2CWONK%2Copsz%2Cwght%5D.ttf'
	uvx --from 'fonttools[woff]' fonttools ttLib.woff2 compress -o public/styles/fonts/Fraunces.woff2 $(FRAUNCES_TTF)
	rm -f $(FRAUNCES_TTF)

.PHONY: fmt
fmt:
	goimports -w -local `head -n 1 go.mod | sed 's/^module //'` .

.PHONY: lint
lint:
	golangci-lint run

# Runs the preview build, which only the preview build tag compiles in: it fills an empty database with a
# made-up office group, "The Loft", timed from now, and lists its people at /preview to sign in as any of
# them. It uses the settings in .env, including DATABASE_PATH and the port, so stop make watch first.
# To seed afresh, delete the database with its -wal and -shm files.
.PHONY: preview
preview:
	go run -tags preview,sqlite_fts5,sqlite_math_functions ./cmd/app

tailwindcss:
	curl -sfL -o tailwindcss https://github.com/tailwindlabs/tailwindcss/releases/latest/download/tailwindcss-macos-arm64
	chmod a+x tailwindcss

.PHONY: test
test: test-up
	AWS_ENDPOINT_URL=http://localhost:$$(./versitygw.sh port versitygw-test) go test -tags sqlite_fts5,sqlite_math_functions -coverprofile cover.out -shuffle on ./...

.PHONY: test-down
test-down:
	./versitygw.sh down

# Starts this checkout's versitygw for the tests in the background, unless it's already running.
# Each checkout gets its own port, so tests in parallel worktrees don't share one.
# It keeps running after the tests, until make test-down.
.PHONY: test-up
test-up:
	@./versitygw.sh up

.PHONY: watch
watch: tailwindcss
	go tool redo
