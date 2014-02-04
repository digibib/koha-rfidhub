all:
	@go vet
	@golint .
	@make todo

build:
	@export GOBIN=$(shell pwd)
	@go build

profile: build
	go test -run none -bench . -benchtime 4s -cpuprofile=prof.out
	go tool pprof ./automathub ./prof.out

run:
	@go run main.go handlers.go config.go rfidunit.go tcp.go ws.go protocols.go --race

todo:
	@grep -rn TODO * || true
	@grep -rn println * || true

test:
	@go test -i
	@go test

integration:
	@go test -tags integration

cover:
	@go test -coverprofile=coverage.out
	@go tool cover -html=coverage.out

clean:
	@go clean
	@rm -f *.out
