.PHONY: build test lint clean

build:
	go build -o alertstoopenclaw .

test:
	CGO_ENABLED=1 go test -v -race ./...

lint:
	golangci-lint run

clean:
	rm -f alertstoopenclaw
