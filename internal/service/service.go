package service

import (
	"context"
	"runtime"

	gov1 "github.com/organic-programming/rob-go/gen/go/go/v1"
	"github.com/organic-programming/rob-go/internal/analyzer"
	"github.com/organic-programming/rob-go/internal/gorunner"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// GoServer implements go.v1.GoService.
type GoServer struct {
	gov1.UnimplementedGoServiceServer
}

func (s *GoServer) Build(_ context.Context, req *gov1.GoCommandRequest) (*gov1.GoCommandResponse, error) {
	return execResponse("build", req), nil
}

func (s *GoServer) Test(_ context.Context, req *gov1.TestRequest) (*gov1.TestResponse, error) {
	args := append([]string{}, req.GetArgs()...)
	if req.GetCover() {
		args = append(args, "-cover")
	}
	if req.GetCoverProfile() != "" {
		args = append(args, "-coverprofile="+req.GetCoverProfile())
	}

	workdir := defaultWorkdir(req.GetWorkdir())
	if req.GetJsonOutput() {
		result, events := gorunner.RunJSON(args, workdir, req.GetEnv(), int(req.GetTimeoutS()))
		stats := summarizeTestEvents(events)
		return &gov1.TestResponse{
			ExitCode: int32(result.ExitCode),
			Stdout:   result.Stdout,
			Stderr:   result.Stderr,
			ElapsedS: float32(result.Elapsed),
			Events:   mapTestEvents(events),
			Total:    int32(stats.total),
			Passed:   int32(stats.passed),
			Failed:   int32(stats.failed),
			Skipped:  int32(stats.skipped),
		}, nil
	}

	result := gorunner.Run("test", args, workdir, req.GetEnv(), int(req.GetTimeoutS()))
	return &gov1.TestResponse{
		ExitCode: int32(result.ExitCode),
		Stdout:   result.Stdout,
		Stderr:   result.Stderr,
		ElapsedS: float32(result.Elapsed),
	}, nil
}

func (s *GoServer) Run(_ context.Context, req *gov1.GoCommandRequest) (*gov1.GoCommandResponse, error) {
	return execResponse("run", req), nil
}

func (s *GoServer) Mod(_ context.Context, req *gov1.GoCommandRequest) (*gov1.GoCommandResponse, error) {
	return execResponse("mod", req), nil
}

func (s *GoServer) Get(_ context.Context, req *gov1.GoCommandRequest) (*gov1.GoCommandResponse, error) {
	return execResponse("get", req), nil
}

func (s *GoServer) Install(_ context.Context, req *gov1.GoCommandRequest) (*gov1.GoCommandResponse, error) {
	return execResponse("install", req), nil
}

func (s *GoServer) Generate(_ context.Context, req *gov1.GoCommandRequest) (*gov1.GoCommandResponse, error) {
	return execResponse("generate", req), nil
}

func (s *GoServer) Clean(_ context.Context, req *gov1.GoCommandRequest) (*gov1.GoCommandResponse, error) {
	return execResponse("clean", req), nil
}

func (s *GoServer) Work(_ context.Context, req *gov1.GoCommandRequest) (*gov1.GoCommandResponse, error) {
	return execResponse("work", req), nil
}

func (s *GoServer) Tool(_ context.Context, req *gov1.GoCommandRequest) (*gov1.GoCommandResponse, error) {
	return execResponse("tool", req), nil
}

func (s *GoServer) Fix(_ context.Context, req *gov1.GoCommandRequest) (*gov1.GoCommandResponse, error) {
	return execResponse("fix", req), nil
}

func (s *GoServer) Env(_ context.Context, req *gov1.GoCommandRequest) (*gov1.GoCommandResponse, error) {
	return execResponse("env", req), nil
}

func (s *GoServer) Format(_ context.Context, req *gov1.FormatRequest) (*gov1.FormatResponse, error) {
	formatted, changed, err := analyzer.Format(req.GetSource(), req.GetFilename())
	if err != nil {
		return &gov1.FormatResponse{
			Formatted: req.GetSource(),
			Changed:   false,
			Error:     err.Error(),
		}, nil
	}
	return &gov1.FormatResponse{Formatted: formatted, Changed: changed}, nil
}

func (s *GoServer) Parse(_ context.Context, req *gov1.ParseRequest) (*gov1.ParseResponse, error) {
	result, err := analyzer.Parse(req.GetSource(), req.GetFilename(), req.GetWithComments())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "parse: %v", err)
	}

	return &gov1.ParseResponse{
		PackageName:  result.PackageName,
		Imports:      result.Imports,
		Declarations: mapDeclarations(result.Declarations),
		Errors:       mapDiagnostics(result.Errors),
	}, nil
}

func (s *GoServer) TypeCheck(_ context.Context, req *gov1.TypeCheckRequest) (*gov1.TypeCheckResponse, error) {
	result, err := analyzer.TypeCheck(req.GetPatterns(), defaultWorkdir(req.GetWorkdir()), req.GetEnv())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "type-check: %v", err)
	}
	return &gov1.TypeCheckResponse{
		Ok:          result.OK,
		Diagnostics: mapDiagnostics(result.Diagnostics),
		Packages:    mapPackages(result.Packages),
	}, nil
}

func (s *GoServer) Analyze(_ context.Context, req *gov1.AnalyzeRequest) (*gov1.AnalyzeResponse, error) {
	diag, err := analyzer.Analyze(req.GetPatterns(), defaultWorkdir(req.GetWorkdir()), req.GetEnv(), req.GetAnalyzers())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "analyze: %v", err)
	}
	return &gov1.AnalyzeResponse{
		Ok:          len(diag) == 0,
		Diagnostics: mapDiagnostics(diag),
	}, nil
}

func (s *GoServer) LoadPackages(_ context.Context, req *gov1.LoadPackagesRequest) (*gov1.LoadPackagesResponse, error) {
	pkgs, err := analyzer.LoadPackages(req.GetPatterns(), defaultWorkdir(req.GetWorkdir()), req.GetEnv(), req.GetWithDeps())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "load-packages: %v", err)
	}
	return &gov1.LoadPackagesResponse{Packages: mapPackages(pkgs)}, nil
}

func (s *GoServer) Doc(_ context.Context, req *gov1.DocRequest) (*gov1.DocResponse, error) {
	doc, err := analyzer.Doc(req.GetPattern(), defaultWorkdir(req.GetWorkdir()))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "doc: %v", err)
	}
	return &gov1.DocResponse{
		PackageName: doc.PackageName,
		PackageDoc:  doc.PackageDoc,
		Symbols:     mapDeclarations(doc.Symbols),
	}, nil
}

func (s *GoServer) Version(context.Context, *gov1.VersionRequest) (*gov1.VersionResponse, error) {
	return &gov1.VersionResponse{
		Version: runtime.Version(),
		Goos:    runtime.GOOS,
		Goarch:  runtime.GOARCH,
		Goroot:  runtime.GOROOT(),
	}, nil
}

func execResponse(subcommand string, req *gov1.GoCommandRequest) *gov1.GoCommandResponse {
	result := gorunner.Run(subcommand, req.GetArgs(), defaultWorkdir(req.GetWorkdir()), req.GetEnv(), int(req.GetTimeoutS()))
	return &gov1.GoCommandResponse{
		ExitCode: int32(result.ExitCode),
		Stdout:   result.Stdout,
		Stderr:   result.Stderr,
		ElapsedS: float32(result.Elapsed),
	}
}

func defaultWorkdir(workdir string) string {
	if workdir == "" {
		return "."
	}
	return workdir
}

func mapDeclarations(in []analyzer.Declaration) []*gov1.Declaration {
	out := make([]*gov1.Declaration, 0, len(in))
	for _, d := range in {
		out = append(out, &gov1.Declaration{
			Kind:      d.Kind,
			Name:      d.Name,
			Signature: d.Signature,
			Doc:       d.Doc,
			Line:      int32(d.Line),
			EndLine:   int32(d.EndLine),
			Exported:  d.Exported,
			Children:  mapDeclarations(d.Children),
		})
	}
	return out
}

func mapDiagnostics(in []analyzer.Diagnostic) []*gov1.Diagnostic {
	out := make([]*gov1.Diagnostic, 0, len(in))
	for _, d := range in {
		out = append(out, &gov1.Diagnostic{
			File:     d.File,
			Line:     int32(d.Line),
			Column:   int32(d.Column),
			Severity: d.Severity,
			Message:  d.Message,
			Category: d.Category,
		})
	}
	return out
}

func mapPackages(in []analyzer.PackageInfo) []*gov1.PackageInfo {
	out := make([]*gov1.PackageInfo, 0, len(in))
	for _, pkg := range in {
		out = append(out, &gov1.PackageInfo{
			Id:      pkg.ID,
			Name:    pkg.Name,
			Dir:     pkg.Dir,
			GoFiles: pkg.GoFiles,
			Imports: pkg.Imports,
			Errors:  mapDiagnostics(pkg.Errors),
		})
	}
	return out
}

type testStats struct {
	total   int
	passed  int
	failed  int
	skipped int
}

func summarizeTestEvents(events []gorunner.TestEvent) testStats {
	stats := testStats{}
	for _, ev := range events {
		if ev.Test == "" {
			continue
		}
		switch ev.Action {
		case "run":
			stats.total++
		case "pass":
			stats.passed++
		case "fail":
			stats.failed++
		case "skip":
			stats.skipped++
		}
	}
	return stats
}

func mapTestEvents(events []gorunner.TestEvent) []*gov1.TestEvent {
	out := make([]*gov1.TestEvent, 0, len(events))
	for _, ev := range events {
		out = append(out, &gov1.TestEvent{
			Time:    ev.Time,
			Action:  ev.Action,
			Package: ev.Package,
			Test:    ev.Test,
			Elapsed: float32(ev.Elapsed),
			Output:  ev.Output,
		})
	}
	return out
}
