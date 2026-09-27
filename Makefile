main_package_path = ./cmd/differ/main.go
binary_name = checkregress

## all: do nothing 
.PHONY: all
all: 
	@echo 'Nothing appropriate'

## help: print this help message
.PHONY: help
help:
	@echo 'Usage:'
	@sed -n 's/^##//p' ${MAKEFILE_LIST} | column -t -s ':' | sed -e 's/^/ /'

.PHONY: confirm
confirm:
	@echo -n 'Are you sure? [y/N] ' && read ans && [ $${ans:-N} = y ]

.PHONY: no-dirty
no-dirty:
	@test -z "$(shell git status --porcelain)"

## audit: run quality control checks
.PHONY: audit
audit: test 
	go mod tidy -diff
	go mod verify
	go vet ./... 
	go run honnef.co/go/tools/cmd/staticcheck@latest -checks=all,-ST1000,-U1000 ./... 
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

## tidy: tidy modfiles and modernize and format .go files
.PHONY: tidy
tidy:
	go mod tidy -v
	go fix ./...
	go fmt ./... 

## test: run all tests
.PHONY: test
test:
	go test -v -race -buildvcs -p 1 ./...

## test/cover: run all tests and display coverage
.PHONY: test/cover
test/cover:
	go test -v -race -buildvcs -p 1 -coverprofile=/tmp/coverage.out ./...
	go tool cover -html=/tmp/coverage.out

## build: build the application
.PHONY: build build/mock
build:
	go build -o=./bin/${binary_name} ${main_package_path}

## run: run the application
.PHONY: run run/mock
run: build
	./bin/${binary_name}


## clean: remove unnecessary files 
.PHONY: clean
clean: 
	rm -rf ./bin 
