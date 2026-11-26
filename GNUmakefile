# test: unit tests plus the Terraform CLI tests against the in-memory fake API (TF_ACC=1, terraform in PATH)
default: build

build:
	go build ./...

test:
	TF_ACC=1 go test ./... -timeout 10m

docs:
	go generate ./...

install:
	go install -ldflags "-X main.version=dev" .
