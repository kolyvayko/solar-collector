BINARY := solar-collector
PKG := ./cmd/solar-collector

.PHONY: build cross test tidy
build:
	CGO_ENABLED=0 go build -o bin/$(BINARY) $(PKG)

cross:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o dist/$(BINARY)-linux-amd64 $(PKG)

test:
	go test ./...

tidy:
	go mod tidy
