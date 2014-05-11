all:
	@go vet
	@golint .
	@make todo

build:
	@go get -u ./...
	@export GOBIN=$(shell pwd)
	@go build

deps:
	go get -d -v ./...
	go list -f '{{range .TestImports}}{{.}} {{end}}' ./... | xargs -n1 go get -d

profile: build
	go test -run none -bench . -cpuprofile=prof.out
	go tool pprof ./koha-rfidhub ./prof.out

run:
	@go run main.go handlers.go config.go rfidunit.go hub.go protocols.go utils.go pool.go sip.go vendors.go metrics.go

todo:
	@grep -rn TODO *.go || true
	@grep -rn println *.go || true

package: build
	tar -cvzf koha-rfidhub.tar.gz koha-rfidhub

test: deps
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
	@rm -f koha-rfidhub*
	@rm -f *.log
