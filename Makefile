GO ?= GO111MODULE=on CGO_ENABLED=0 go
GO_TEST ?= CGO_ENABLED=1 go
GOOS ?= $(shell $(GO) version | cut -d' ' -f4 | cut -d'/' -f1)
GOARCH ?= $(shell $(GO) version | cut -d' ' -f4 | cut -d'/' -f2)

CMD_GO_FILES ?= ./cmd/yafe/main.go

export GO111MODULE=on

PNPM ?= pnpm
GOSEC ?= gosec
GRYPE ?= grype
DOCKER ?= docker

BIN_DIR = $(shell pwd)/bin
TEST_DIR = $(shell pwd)/coverage
WEB_DIR = $(shell pwd)/internal/frontend/app
WEB_BUILD_DIR = $(shell pwd)/internal/frontend/app/dist
WEB_NODE_DIR = $(shell pwd)/internal/frontend/app/node_modules
WEB_COVERAGE_DIR = $(shell pwd)/internal/frontend/app/coverage
WEB_CI_DIR = $(shell pwd)/internal/frontend/app/ci/*.xml

clean: clean-server clean-web
clean-server:
	@rm -rf ${BIN_DIR}
	@rm -rf ${TEST_DIR}
	@rm -rf coverage.out
	@$(GO) clean -testcache
clean-web:
	@rm -rf ${WEB_BUILD_DIR}
	@rm -rf ${WEB_NODE_DIR}
	@rm -rf ${WEB_COVERAGE_DIR}
	@rm -rf ${WEB_CI_DIR}
	@rm -f ${WEB_DIR}/.eslintcache

dependencies: dependencies-web dependencies-server
dependencies-server:
	$(GO) mod download
dependencies-web:
	cd ${WEB_DIR}; $(PNPM) install --frozen-lockfile

checkstyle: checkstyle-web checkstyle-server
checkstyle-server:
	$(GO) vet ./...
checkstyle-web:
	cd ${WEB_DIR}; $(PNPM) checkstyle

generate:
	$(GO) generate ./...

test:
	$(GO_TEST) test -race -shuffle on -v ./...
test-coverage:
	@make clean
	@mkdir -p ${TEST_DIR}
	$(GO_TEST) test -coverprofile ${TEST_DIR}/coverage.out -race -shuffle on -v ./...
	@grep -v -E "dto.go|enum.go|_generated.go|_test.go|main.go" ${TEST_DIR}/coverage.out > ${TEST_DIR}/coverage.final.out || true
	$(GO_TEST) tool cover -func=${TEST_DIR}/coverage.final.out

audit: audit-web audit-server
audit-server:
	$(GOSEC) -quiet -sort -severity medium -confidence high ./...
audit-web:
	cd ${WEB_DIR}; $(PNPM) audit -P --audit-level critical;

scan:
	@NO_COLOR=1 $(GRYPE) -v -o table --file bin/grype.txt --fail-on critical bin/ || true
	@cat ./bin/grype.txt

build: clean dependencies build-web
	$(GO) build -tags embed -o ${BIN_DIR}/yafe-${GOOS}-${GOARCH} ${CMD_GO_FILES}
build-web: dependencies-web
	cd ${WEB_DIR}; $(PNPM) build

build-all: build-freebsd-amd64 build-freebsd-arm64 build-darwin-amd64 build-darwin-arm64 build-linux-amd64 build-linux-arm64

build-freebsd-amd64: build-web
	GOOS=freebsd GOARCH=amd64 $(GO) build -tags embed -o ${BIN_DIR}/yafe-freebsd-amd64 ${CMD_GO_FILES}
build-freebsd-arm64: build-web
	GOOS=freebsd GOARCH=arm64 $(GO) build -tags embed -o ${BIN_DIR}/yafe-freebsd-arm64 ${CMD_GO_FILES}
build-darwin-amd64: build-web
	GOOS=darwin GOARCH=amd64 $(GO) build -tags embed -o ${BIN_DIR}/yafe-darwin-amd64 ${CMD_GO_FILES}
build-darwin-arm64: build-web
	GOOS=darwin GOARCH=arm64 $(GO) build -tags embed -o ${BIN_DIR}/yafe-darwin-arm64 ${CMD_GO_FILES}
build-linux-amd64: build-web
	GOOS=linux GOARCH=amd64 $(GO) build -tags embed -o ${BIN_DIR}/yafe-linux-amd64 ${CMD_GO_FILES}
build-linux-arm64: build-web
	GOOS=linux GOARCH=arm64 $(GO) build -tags embed -o ${BIN_DIR}/yafe-linux-arm64 ${CMD_GO_FILES}

dev-server: clean dependencies
	$(GO) run ${CMD_GO_FILES} serve

dev-web: clean dependencies
	cd ${WEB_DIR}; $(PNPM) start

.PHONY: clean clean-server clean-web \
	dependencies dependencies-server dependencies-web \
	checkstyle checkstyle-server checkstyle-web \
	generate \
	test test-coverage \
	audit audit-server audit-web \
	scan \
	build build-web build-all \
	build-freebsd-amd64 build-freebsd-arm64 \
	build-darwin-amd64 build-darwin-arm64 \
	build-linux-amd64 build-linux-arm64 \
	dev-server \
	dev-web