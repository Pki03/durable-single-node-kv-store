.PHONY: build test bench recovery clean

build:
	go build -o bin/kvprjt ./cmd/kvprjt

test:
	go test ./... -v -count=1

bench: build
	./bin/kvprjt -dir ./data -bench

recovery: build
	./bin/kvprjt -dir ./data -recover

harness: build
	./bin/kvprjt -dir ./data -harness

clean:
	rm -rf ./data ./bin

go-mod:
	go mod tidy

all: build test bench
