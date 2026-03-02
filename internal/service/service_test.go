package service

import (
	"context"
	"testing"
	"time"

	"github.com/organic-programming/go-holons/pkg/grpcclient"
	"github.com/organic-programming/go-holons/pkg/transport"
	gov1 "github.com/organic-programming/rob-go/gen/go/go/v1"
	"google.golang.org/grpc"
)

func startTestServer(t *testing.T) (gov1.GoServiceClient, func()) {
	t.Helper()

	mem := transport.NewMemListener()
	srv := grpc.NewServer()
	gov1.RegisterGoServiceServer(srv, &GoServer{})
	go func() { _ = srv.Serve(mem) }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := grpcclient.DialMem(ctx, mem)
	if err != nil {
		t.Fatalf("DialMem: %v", err)
	}

	cleanup := func() {
		_ = conn.Close()
		srv.Stop()
		_ = mem.Close()
	}

	return gov1.NewGoServiceClient(conn), cleanup
}

func TestVersion(t *testing.T) {
	client, cleanup := startTestServer(t)
	defer cleanup()

	resp, err := client.Version(context.Background(), &gov1.VersionRequest{})
	if err != nil {
		t.Fatalf("Version error: %v", err)
	}
	if resp.GetVersion() == "" || resp.GetGoos() == "" || resp.GetGoarch() == "" {
		t.Fatalf("invalid version response: %+v", resp)
	}
}

func TestFormat(t *testing.T) {
	client, cleanup := startTestServer(t)
	defer cleanup()

	resp, err := client.Format(context.Background(), &gov1.FormatRequest{
		Source: "package main\nfunc main(){}\n",
	})
	if err != nil {
		t.Fatalf("Format error: %v", err)
	}
	if !resp.GetChanged() {
		t.Fatalf("expected changed=true, resp=%+v", resp)
	}
}

func TestParse(t *testing.T) {
	client, cleanup := startTestServer(t)
	defer cleanup()

	resp, err := client.Parse(context.Background(), &gov1.ParseRequest{
		Source: "package x\n\nfunc A() {}\n",
	})
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if resp.GetPackageName() != "x" {
		t.Fatalf("package=%q, want x", resp.GetPackageName())
	}
	if len(resp.GetDeclarations()) == 0 {
		t.Fatalf("expected declarations, got %+v", resp)
	}
}
