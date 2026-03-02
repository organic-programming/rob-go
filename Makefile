.PHONY: build test proto clean

build:
	go build -o rob-go ./cmd/rob-go

test:
	go test ./... -v -race

proto:
	protoc \
		--go_out=. --go_opt=module=github.com/organic-programming/rob-go \
		--go-grpc_out=. --go-grpc_opt=module=github.com/organic-programming/rob-go \
		protos/go/v1/go.proto

clean:
	rm -f rob-go
	go clean -cache
