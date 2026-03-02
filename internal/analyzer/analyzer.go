package analyzer

import (
	"fmt"
	"go/ast"
	"go/doc"
	"go/format"
	"go/parser"
	"go/scanner"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/assign"
	"golang.org/x/tools/go/analysis/passes/atomic"
	"golang.org/x/tools/go/analysis/passes/bools"
	"golang.org/x/tools/go/analysis/passes/buildtag"
	"golang.org/x/tools/go/analysis/passes/copylock"
	"golang.org/x/tools/go/analysis/passes/loopclosure"
	"golang.org/x/tools/go/analysis/passes/nilfunc"
	"golang.org/x/tools/go/analysis/passes/printf"
	"golang.org/x/tools/go/analysis/passes/shift"
	"golang.org/x/tools/go/analysis/passes/stdmethods"
	"golang.org/x/tools/go/analysis/passes/structtag"
	"golang.org/x/tools/go/analysis/passes/tests"
	"golang.org/x/tools/go/analysis/passes/unreachable"
	"golang.org/x/tools/go/analysis/passes/unusedresult"
	"golang.org/x/tools/go/packages"
)

// ParseResult is the structural output of Parse.
type ParseResult struct {
	PackageName  string
	Imports      []string
	Declarations []Declaration
	Errors       []Diagnostic
}

// TypeCheckResult is the output of TypeCheck.
type TypeCheckResult struct {
	OK          bool
	Diagnostics []Diagnostic
	Packages    []PackageInfo
}

// DocResult is the output of Doc.
type DocResult struct {
	PackageName string
	PackageDoc  string
	Symbols     []Declaration
}

// Declaration is a flattened AST/doc declaration.
type Declaration struct {
	Kind      string
	Name      string
	Signature string
	Doc       string
	Line      int
	EndLine   int
	Exported  bool
	Children  []Declaration
}

// Diagnostic is a structured analysis/type-check message.
type Diagnostic struct {
	File     string
	Line     int
	Column   int
	Severity string
	Message  string
	Category string
}

// PackageInfo summarizes one loaded package.
type PackageInfo struct {
	ID      string
	Name    string
	Dir     string
	GoFiles []string
	Imports []string
	Errors  []Diagnostic
}

// Format formats Go source in-memory.
func Format(source, _ string) (string, bool, error) {
	formatted, err := format.Source([]byte(source))
	if err != nil {
		return "", false, err
	}
	out := string(formatted)
	return out, out != source, nil
}

// Parse parses Go source and extracts declarations.
func Parse(source, filename string, withComments bool) (*ParseResult, error) {
	if filename == "" {
		filename = "input.go"
	}

	mode := parser.AllErrors
	if withComments {
		mode |= parser.ParseComments
	}

	fset := token.NewFileSet()
	res := &ParseResult{}
	file, err := parser.ParseFile(fset, filename, source, mode)
	if err != nil {
		res.Errors = append(res.Errors, parseErrorDiagnostics(err)...)
	}
	if file == nil {
		return res, nil
	}

	res.PackageName = file.Name.Name
	res.Imports = extractImports(file)
	res.Declarations = extractDecls(file, fset)
	return res, nil
}

// TypeCheck loads and type-checks packages.
func TypeCheck(patterns []string, workdir string, env []string) (*TypeCheckResult, error) {
	pkgs, err := loadWithConfig(patterns, workdir, env,
		packages.NeedName|
			packages.NeedFiles|
			packages.NeedCompiledGoFiles|
			packages.NeedImports|
			packages.NeedDeps|
			packages.NeedTypes|
			packages.NeedTypesSizes|
			packages.NeedSyntax|
			packages.NeedTypesInfo,
	)
	if err != nil {
		return nil, err
	}

	diag := make([]Diagnostic, 0)
	infos := make([]PackageInfo, 0, len(pkgs))
	for _, pkg := range pkgs {
		pkgDiag := packageErrors(pkg.Errors, "typecheck")
		diag = append(diag, pkgDiag...)
		infos = append(infos, packageInfo(pkg, pkgDiag))
	}

	return &TypeCheckResult{
		OK:          len(diag) == 0,
		Diagnostics: diag,
		Packages:    infos,
	}, nil
}

// Analyze runs static analysis via go/analysis over loaded packages.
func Analyze(patterns []string, workdir string, env []string, analyzerNames []string) ([]Diagnostic, error) {
	pkgs, err := loadWithConfig(patterns, workdir, env,
		packages.NeedName|
			packages.NeedFiles|
			packages.NeedCompiledGoFiles|
			packages.NeedImports|
			packages.NeedDeps|
			packages.NeedTypes|
			packages.NeedTypesSizes|
			packages.NeedSyntax|
			packages.NeedTypesInfo,
	)
	if err != nil {
		return nil, err
	}

	selected, err := selectAnalyzers(analyzerNames)
	if err != nil {
		return nil, err
	}

	diagnostics := make([]Diagnostic, 0)
	for _, pkg := range pkgs {
		diagnostics = append(diagnostics, packageErrors(pkg.Errors, "typecheck")...)
		if pkg.Types == nil || len(pkg.Syntax) == 0 {
			continue
		}
		for _, a := range selected {
			ds, runErr := runAnalyzer(pkg, a)
			diagnostics = append(diagnostics, ds...)
			if runErr != nil {
				diagnostics = append(diagnostics, Diagnostic{
					Severity: "error",
					Message:  runErr.Error(),
					Category: a.Name,
				})
			}
		}
	}

	return diagnostics, nil
}

// LoadPackages loads package metadata and dependency graph.
func LoadPackages(patterns []string, workdir string, env []string, withDeps bool) ([]PackageInfo, error) {
	mode := packages.NeedName |
		packages.NeedFiles |
		packages.NeedCompiledGoFiles |
		packages.NeedImports
	if withDeps {
		mode |= packages.NeedDeps
	}

	roots, err := loadWithConfig(patterns, workdir, env, mode)
	if err != nil {
		return nil, err
	}

	pkgs := roots
	if withDeps {
		pkgs = collectWithDeps(roots)
	}

	infos := make([]PackageInfo, 0, len(pkgs))
	for _, pkg := range pkgs {
		infos = append(infos, packageInfo(pkg, packageErrors(pkg.Errors, "load")))
	}

	sort.Slice(infos, func(i, j int) bool {
		return infos[i].ID < infos[j].ID
	})

	return infos, nil
}

// Doc extracts documentation from packages.
func Doc(pattern, workdir string) (*DocResult, error) {
	if strings.TrimSpace(pattern) == "" {
		pattern = "."
	}

	pkgs, err := loadWithConfig([]string{pattern}, workdir, nil,
		packages.NeedName|
			packages.NeedFiles|
			packages.NeedCompiledGoFiles|
			packages.NeedSyntax,
	)
	if err != nil {
		return nil, err
	}
	if len(pkgs) == 0 {
		return nil, fmt.Errorf("no packages found for pattern %q", pattern)
	}

	pkg := pkgs[0]
	if pkg.Fset == nil {
		pkg.Fset = token.NewFileSet()
	}

	docPkg, err := doc.NewFromFiles(pkg.Fset, pkg.Syntax, pkg.PkgPath, doc.AllDecls)
	if err != nil {
		return nil, err
	}

	result := &DocResult{
		PackageName: docPkg.Name,
		PackageDoc:  strings.TrimSpace(docPkg.Doc),
		Symbols:     make([]Declaration, 0),
	}

	for _, f := range docPkg.Funcs {
		result.Symbols = append(result.Symbols, declarationFromFunc(pkg.Fset, f.Decl, strings.TrimSpace(f.Doc)))
	}
	for _, t := range docPkg.Types {
		decl := declarationFromTypeDoc(pkg.Fset, t)
		result.Symbols = append(result.Symbols, decl)
	}
	for _, c := range docPkg.Consts {
		result.Symbols = append(result.Symbols, declarationFromValueDoc(pkg.Fset, "const", c)...)
	}
	for _, v := range docPkg.Vars {
		result.Symbols = append(result.Symbols, declarationFromValueDoc(pkg.Fset, "var", v)...)
	}

	sort.Slice(result.Symbols, func(i, j int) bool {
		return result.Symbols[i].Name < result.Symbols[j].Name
	})

	return result, nil
}

func extractImports(file *ast.File) []string {
	imports := make([]string, 0, len(file.Imports))
	for _, imp := range file.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			imports = append(imports, imp.Path.Value)
			continue
		}
		imports = append(imports, path)
	}
	return imports
}

func extractDecls(file *ast.File, fset *token.FileSet) []Declaration {
	decls := make([]Declaration, 0)
	for _, d := range file.Decls {
		switch decl := d.(type) {
		case *ast.FuncDecl:
			decls = append(decls, declarationFromFunc(fset, decl, strings.TrimSpace(docFromGroup(decl.Doc))))
		case *ast.GenDecl:
			decls = append(decls, declarationFromGenDecl(fset, decl)...)
		}
	}
	return decls
}

func declarationFromFunc(fset *token.FileSet, decl *ast.FuncDecl, docText string) Declaration {
	line, endLine := lineRange(fset, decl.Pos(), decl.End())
	sig := formatNode(fset, decl.Type)
	sig = strings.TrimPrefix(sig, "func")
	if decl.Recv != nil && len(decl.Recv.List) > 0 {
		recv := formatFieldList(fset, decl.Recv)
		sig = fmt.Sprintf("func (%s) %s%s", recv, decl.Name.Name, sig)
	} else {
		sig = fmt.Sprintf("func %s%s", decl.Name.Name, sig)
	}

	return Declaration{
		Kind:      "func",
		Name:      decl.Name.Name,
		Signature: strings.TrimSpace(sig),
		Doc:       docText,
		Line:      line,
		EndLine:   endLine,
		Exported:  decl.Name.IsExported(),
	}
}

func declarationFromGenDecl(fset *token.FileSet, decl *ast.GenDecl) []Declaration {
	out := make([]Declaration, 0)
	for _, spec := range decl.Specs {
		switch s := spec.(type) {
		case *ast.TypeSpec:
			line, endLine := lineRange(fset, s.Pos(), s.End())
			d := Declaration{
				Kind:      "type",
				Name:      s.Name.Name,
				Signature: formatNode(fset, s.Type),
				Doc:       strings.TrimSpace(firstNonEmpty(docFromGroup(s.Doc), docFromGroup(decl.Doc))),
				Line:      line,
				EndLine:   endLine,
				Exported:  s.Name.IsExported(),
				Children:  typeChildren(fset, s.Type),
			}
			out = append(out, d)
		case *ast.ValueSpec:
			kind := strings.ToLower(decl.Tok.String())
			docText := strings.TrimSpace(firstNonEmpty(docFromGroup(s.Doc), docFromGroup(decl.Doc)))
			signature := ""
			if s.Type != nil {
				signature = formatNode(fset, s.Type)
			}
			line, endLine := lineRange(fset, s.Pos(), s.End())
			for _, n := range s.Names {
				out = append(out, Declaration{
					Kind:      kind,
					Name:      n.Name,
					Signature: signature,
					Doc:       docText,
					Line:      line,
					EndLine:   endLine,
					Exported:  n.IsExported(),
				})
			}
		}
	}
	return out
}

func declarationFromTypeDoc(fset *token.FileSet, t *doc.Type) Declaration {
	line, endLine := lineRange(fset, t.Decl.Pos(), t.Decl.End())
	decl := Declaration{
		Kind:      "type",
		Name:      t.Name,
		Signature: "",
		Doc:       strings.TrimSpace(t.Doc),
		Line:      line,
		EndLine:   endLine,
		Exported:  ast.IsExported(t.Name),
		Children:  make([]Declaration, 0),
	}

	if len(t.Decl.Specs) > 0 {
		if ts, ok := t.Decl.Specs[0].(*ast.TypeSpec); ok {
			decl.Signature = formatNode(fset, ts.Type)
		}
	}

	for _, m := range t.Methods {
		decl.Children = append(decl.Children, declarationFromFunc(fset, m.Decl, strings.TrimSpace(m.Doc)))
	}
	return decl
}

func declarationFromValueDoc(fset *token.FileSet, kind string, v *doc.Value) []Declaration {
	out := make([]Declaration, 0, len(v.Names))
	line, endLine := lineRange(fset, v.Decl.Pos(), v.Decl.End())
	for _, name := range v.Names {
		out = append(out, Declaration{
			Kind:     kind,
			Name:     name,
			Doc:      strings.TrimSpace(v.Doc),
			Line:     line,
			EndLine:  endLine,
			Exported: ast.IsExported(name),
		})
	}
	return out
}

func typeChildren(fset *token.FileSet, expr ast.Expr) []Declaration {
	children := make([]Declaration, 0)
	switch t := expr.(type) {
	case *ast.StructType:
		for _, f := range t.Fields.List {
			line, endLine := lineRange(fset, f.Pos(), f.End())
			typ := formatNode(fset, f.Type)
			if len(f.Names) == 0 {
				name := typ
				children = append(children, Declaration{
					Kind:      "field",
					Name:      name,
					Signature: typ,
					Doc:       strings.TrimSpace(docFromGroup(f.Doc)),
					Line:      line,
					EndLine:   endLine,
					Exported:  ast.IsExported(name),
				})
				continue
			}
			for _, name := range f.Names {
				children = append(children, Declaration{
					Kind:      "field",
					Name:      name.Name,
					Signature: typ,
					Doc:       strings.TrimSpace(docFromGroup(f.Doc)),
					Line:      line,
					EndLine:   endLine,
					Exported:  name.IsExported(),
				})
			}
		}
	case *ast.InterfaceType:
		for _, f := range t.Methods.List {
			line, endLine := lineRange(fset, f.Pos(), f.End())
			typ := formatNode(fset, f.Type)
			if len(f.Names) == 0 {
				children = append(children, Declaration{
					Kind:      "method",
					Name:      typ,
					Signature: typ,
					Doc:       strings.TrimSpace(docFromGroup(f.Doc)),
					Line:      line,
					EndLine:   endLine,
					Exported:  ast.IsExported(typ),
				})
				continue
			}
			for _, name := range f.Names {
				children = append(children, Declaration{
					Kind:      "method",
					Name:      name.Name,
					Signature: typ,
					Doc:       strings.TrimSpace(docFromGroup(f.Doc)),
					Line:      line,
					EndLine:   endLine,
					Exported:  name.IsExported(),
				})
			}
		}
	}
	return children
}

func lineRange(fset *token.FileSet, start token.Pos, end token.Pos) (int, int) {
	sp := fset.Position(start)
	ep := fset.Position(end)
	return sp.Line, ep.Line
}

func formatNode(fset *token.FileSet, node any) string {
	if node == nil {
		return ""
	}
	var b strings.Builder
	if err := format.Node(&b, fset, node); err != nil {
		return ""
	}
	return b.String()
}

func formatFieldList(fset *token.FileSet, fl *ast.FieldList) string {
	if fl == nil {
		return ""
	}
	parts := make([]string, 0)
	for _, f := range fl.List {
		typ := formatNode(fset, f.Type)
		if len(f.Names) == 0 {
			parts = append(parts, typ)
			continue
		}
		for _, n := range f.Names {
			parts = append(parts, n.Name+" "+typ)
		}
	}
	return strings.Join(parts, ", ")
}

func docFromGroup(group *ast.CommentGroup) string {
	if group == nil {
		return ""
	}
	return strings.TrimSpace(group.Text())
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func parseErrorDiagnostics(err error) []Diagnostic {
	if err == nil {
		return nil
	}
	out := make([]Diagnostic, 0)
	if list, ok := err.(scanner.ErrorList); ok {
		for _, item := range list {
			out = append(out, Diagnostic{
				File:     item.Pos.Filename,
				Line:     item.Pos.Line,
				Column:   item.Pos.Column,
				Severity: "error",
				Message:  item.Msg,
				Category: "parse",
			})
		}
		return out
	}
	file, line, col := parsePos(err.Error())
	out = append(out, Diagnostic{
		File:     file,
		Line:     line,
		Column:   col,
		Severity: "error",
		Message:  err.Error(),
		Category: "parse",
	})
	return out
}

func loadWithConfig(patterns []string, workdir string, env []string, mode packages.LoadMode) ([]*packages.Package, error) {
	if len(patterns) == 0 {
		patterns = []string{"./..."}
	}
	if strings.TrimSpace(workdir) == "" {
		workdir = "."
	}
	cfg := &packages.Config{
		Mode: mode,
		Dir:  workdir,
		Env:  append(os.Environ(), env...),
		Fset: token.NewFileSet(),
	}
	return packages.Load(cfg, patterns...)
}

func packageErrors(errs []packages.Error, category string) []Diagnostic {
	out := make([]Diagnostic, 0, len(errs))
	for _, e := range errs {
		file, line, col := parsePos(e.Pos)
		out = append(out, Diagnostic{
			File:     file,
			Line:     line,
			Column:   col,
			Severity: "error",
			Message:  e.Msg,
			Category: firstNonEmpty(category, fmt.Sprintf("%v", e.Kind)),
		})
	}
	return out
}

func packageInfo(pkg *packages.Package, errs []Diagnostic) PackageInfo {
	imports := make([]string, 0, len(pkg.Imports))
	for path := range pkg.Imports {
		imports = append(imports, path)
	}
	sort.Strings(imports)

	dir := ""
	if len(pkg.GoFiles) > 0 {
		dir = filepath.Dir(pkg.GoFiles[0])
	}

	return PackageInfo{
		ID:      pkg.ID,
		Name:    pkg.Name,
		Dir:     dir,
		GoFiles: append([]string{}, pkg.GoFiles...),
		Imports: imports,
		Errors:  errs,
	}
}

func parsePos(pos string) (string, int, int) {
	if strings.TrimSpace(pos) == "" {
		return "", 0, 0
	}
	parts := strings.Split(pos, ":")
	if len(parts) < 2 {
		return pos, 0, 0
	}

	col := 0
	line := 0
	fileParts := parts

	if n, err := strconv.Atoi(parts[len(parts)-1]); err == nil {
		col = n
		fileParts = parts[:len(parts)-1]
	}
	if len(fileParts) > 0 {
		if n, err := strconv.Atoi(fileParts[len(fileParts)-1]); err == nil {
			line = n
			fileParts = fileParts[:len(fileParts)-1]
		}
	}

	file := strings.Join(fileParts, ":")
	return file, line, col
}

func collectWithDeps(roots []*packages.Package) []*packages.Package {
	seen := make(map[string]*packages.Package)
	var walk func(*packages.Package)
	walk = func(pkg *packages.Package) {
		if pkg == nil {
			return
		}
		if _, ok := seen[pkg.ID]; ok {
			return
		}
		seen[pkg.ID] = pkg
		for _, imp := range pkg.Imports {
			walk(imp)
		}
	}
	for _, r := range roots {
		walk(r)
	}
	out := make([]*packages.Package, 0, len(seen))
	for _, pkg := range seen {
		out = append(out, pkg)
	}
	return out
}

func selectAnalyzers(requested []string) ([]*analysis.Analyzer, error) {
	all := map[string]*analysis.Analyzer{
		assign.Analyzer.Name:       assign.Analyzer,
		atomic.Analyzer.Name:       atomic.Analyzer,
		bools.Analyzer.Name:        bools.Analyzer,
		buildtag.Analyzer.Name:     buildtag.Analyzer,
		copylock.Analyzer.Name:     copylock.Analyzer,
		loopclosure.Analyzer.Name:  loopclosure.Analyzer,
		nilfunc.Analyzer.Name:      nilfunc.Analyzer,
		printf.Analyzer.Name:       printf.Analyzer,
		shift.Analyzer.Name:        shift.Analyzer,
		stdmethods.Analyzer.Name:   stdmethods.Analyzer,
		structtag.Analyzer.Name:    structtag.Analyzer,
		tests.Analyzer.Name:        tests.Analyzer,
		unreachable.Analyzer.Name:  unreachable.Analyzer,
		unusedresult.Analyzer.Name: unusedresult.Analyzer,
	}

	if len(requested) == 0 {
		keys := make([]string, 0, len(all))
		for name := range all {
			keys = append(keys, name)
		}
		sort.Strings(keys)
		selected := make([]*analysis.Analyzer, 0, len(keys))
		for _, name := range keys {
			selected = append(selected, all[name])
		}
		return selected, nil
	}

	selected := make([]*analysis.Analyzer, 0, len(requested))
	for _, name := range requested {
		a, ok := all[name]
		if !ok {
			return nil, fmt.Errorf("unknown analyzer %q", name)
		}
		selected = append(selected, a)
	}
	return selected, nil
}

func runAnalyzer(pkg *packages.Package, target *analysis.Analyzer) ([]Diagnostic, error) {
	results := make(map[*analysis.Analyzer]any)
	diagnostics := make([]Diagnostic, 0)

	var run func(a *analysis.Analyzer) (any, error)
	run = func(a *analysis.Analyzer) (result any, err error) {
		if cached, ok := results[a]; ok {
			return cached, nil
		}

		deps := make(map[*analysis.Analyzer]any, len(a.Requires))
		for _, req := range a.Requires {
			dep, depErr := run(req)
			if depErr != nil {
				return nil, depErr
			}
			deps[req] = dep
		}

		pass := &analysis.Pass{
			Analyzer:   a,
			Fset:       pkg.Fset,
			Files:      pkg.Syntax,
			OtherFiles: pkg.OtherFiles,
			Pkg:        pkg.Types,
			TypesInfo:  pkg.TypesInfo,
			TypesSizes: pkg.TypesSizes,
			ResultOf:   deps,
			Report: func(d analysis.Diagnostic) {
				diagnostics = append(diagnostics, analysisDiagnostic(pkg.Fset, d, a.Name))
			},
			ImportObjectFact:  func(types.Object, analysis.Fact) bool { return false },
			ExportObjectFact:  func(types.Object, analysis.Fact) {},
			ImportPackageFact: func(*types.Package, analysis.Fact) bool { return false },
			ExportPackageFact: func(analysis.Fact) {},
			AllObjectFacts:    func() []analysis.ObjectFact { return nil },
			AllPackageFacts:   func() []analysis.PackageFact { return nil },
		}

		defer func() {
			if rec := recover(); rec != nil {
				err = fmt.Errorf("analyzer %s panic: %v", a.Name, rec)
			}
		}()

		result, err = a.Run(pass)
		if err != nil {
			return nil, err
		}

		results[a] = result
		return result, nil
	}

	_, err := run(target)
	return diagnostics, err
}

func analysisDiagnostic(fset *token.FileSet, d analysis.Diagnostic, category string) Diagnostic {
	pos := fset.Position(d.Pos)
	cat := category
	if strings.TrimSpace(d.Category) != "" {
		cat = d.Category
	}
	return Diagnostic{
		File:     pos.Filename,
		Line:     pos.Line,
		Column:   pos.Column,
		Severity: "warning",
		Message:  d.Message,
		Category: cat,
	}
}
