default: build

build:
	go build -o terraform-provider-buildkite .

fmt:
	go fmt ./...

testfmt:
	@test -z $(shell gofmt -l . buildkite | tee /dev/stderr) || echo "[WARN] Fix formatting issues with 'make fmt'"

vet:
	go vet $(go list ./...)

test:
	go test ./...
