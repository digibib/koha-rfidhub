all:
	@go vet
	@golint .
	@make todo

build:
	@export GOBIN=$(shell pwd)
	@go build

profile: build
	go test -run none -bench . -cpuprofile=prof.out
	go tool pprof ./koha-rfidhub ./prof.out

run:
	@go run main.go handlers.go config.go rfidunit.go tcp.go ws.go protocols.go --race

todo:
	@grep -rn TODO * || true
	@grep -rn println * || true

test:
	@go test -i
	@go test --race

integration:
	@go test -tags integration

cover:
	@go test -coverprofile=coverage.out
	@go tool cover -html=coverage.out

clean:
	@go clean
	@rm -f *.out
	@rm -f koha-rfidhub*
