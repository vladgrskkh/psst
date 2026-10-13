# =========================================================================================== #
# HELPERS
# =========================================================================================== #

## help: print this help message
.PHONY: help
help:
	@echo 'Usage:'
	@sed -n 's/^##//p' ${MAKEFILE_LIST} | column -t -s ':' | sed -e 's/^/ /'

.PHONY: confirm
confirm:
	@echo 'Are you sure? [y/N] ' && read ans && [ $${ans:-N} = y ]

# =========================================================================================== #
# DEVELOPMENT
# =========================================================================================== #

## run/psst: run the psst CLI
.PHONY: run/psst
run/psst:
	go run ./cmd/psst

# =========================================================================================== #
# QUALITY CONTROL
# =========================================================================================== #

## audit: tidy and vendor dependencies and format, vet and test all code
.PHONY: audit
audit:
	@echo 'Tidying and verifying module dependencies...'
	go mod tidy
	go mod verify
	@echo 'Formatting code...'
	go fmt ./...
	@echo 'Vetting code...'
	go vet ./...
	golangci-lint run
	@echo 'Running tests...'
	go test -race -vet=off ./...

## vendor: vendor dependencies
.PHONY: vendor
vendor:
	@echo 'Vendoring dependencies...'
	go mod vendor

# =========================================================================================== #
# BUILD
# =========================================================================================== #

## build/psst: build the psst CLI
.PHONY: build/psst
build/psst:
	go build -o=./bin/psst ./cmd/psst

## build/bridge: build the macOS reminders bridge (Swift/EventKit)
.PHONY: build/bridge
build/bridge:
	mkdir -p ./bin
	swiftc -O -o ./bin/psst-bridge ./internal/infra/reminders/macosbridge/main.swift
