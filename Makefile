APP_NAME := alertstoopenclaw

.PHONY: build test lint clean fmt vet check

## build: Compile the application binary.
build:
	go build -o $(APP_NAME) .

## fmt: Format all Go source files.
fmt:
	gofmt -w .

## vet: Run go vet on all packages.
vet:
	go vet ./...

## lint: Run golangci-lint with strict configuration.
lint:
	golangci-lint run

## test: Run all tests with race detector and coverage.
test:
	CGO_ENABLED=1 go test -v -race -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

## check: Run fmt, vet, lint, and test in sequence.
check: fmt vet lint test

## clean: Remove build artifacts and coverage output.
clean:
	rm -f $(APP_NAME) coverage.out
