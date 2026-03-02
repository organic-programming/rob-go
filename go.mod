module github.com/organic-programming/rob-go

go 1.24.0

require (
	github.com/organic-programming/go-holons v0.2.1
	golang.org/x/tools v0.38.0
	google.golang.org/grpc v1.78.0
	google.golang.org/protobuf v1.36.11
)

require (
	golang.org/x/mod v0.29.0 // indirect
	golang.org/x/net v0.47.0 // indirect
	golang.org/x/sync v0.18.0 // indirect
	golang.org/x/sys v0.38.0 // indirect
	golang.org/x/text v0.31.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20251029180050-ab9386a59fda // indirect
	nhooyr.io/websocket v1.8.17 // indirect
)

replace github.com/organic-programming/go-holons => ../../sdk/go-holons
