package main

import (
	"fmt"
	"os"

	"github.com/organic-programming/go-holons/pkg/serve"
	gov1 "github.com/organic-programming/rob-go/gen/go/go/v1"
	"github.com/organic-programming/rob-go/internal/service"
	"google.golang.org/grpc"
)

func main() {
	listenURI := serve.ParseFlags(os.Args[1:])
	if err := serve.Run(listenURI, func(s *grpc.Server) {
		gov1.RegisterGoServiceServer(s, &service.GoServer{})
	}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
