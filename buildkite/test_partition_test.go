package buildkite

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
)

// TestHarnessPartition checks the naming boundary used to select acceptance tests.
// It checks direct Terraform harness usage in this directory, including closures,
// not whether arbitrary Go tests or their dependencies can access the network.
func TestHarnessPartition(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate test source directory")
	}
	dir := filepath.Dir(filename)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	files := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		files++
		path := filepath.Join(dir, entry.Name())
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Error(err)
			continue
		}
		for _, violation := range harnessPartitionViolations(fset, file) {
			t.Error(violation)
		}
	}
	t.Logf("checked %d test source files", files)
}

func harnessPartitionViolations(fset *token.FileSet, file *ast.File) []string {
	const resourcePath = "github.com/hashicorp/terraform-plugin-testing/helper/resource"
	var violations []string
	report := func(node ast.Node, message string) {
		violations = append(violations, fmt.Sprintf("%s: %s", fset.Position(node.Pos()), message))
	}
	resourceAliases := map[string]bool{}
	testingAliases := map[string]bool{}
	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			continue // The parser already validates import string syntax.
		}
		alias := filepath.Base(path)
		if spec.Name != nil {
			alias = spec.Name.Name
		}
		switch path {
		case resourcePath:
			if alias == "." {
				report(spec, "dot imports of the Terraform resource harness bypass the naming guard")
			}
			resourceAliases[alias] = true
		case "testing":
			testingAliases[alias] = true
		}
	}

	// Only direct calls are allowed, so aliases or wrappers cannot hide a harness
	// call's top-level owner. Parentheses around the function are harmless.
	directCalls := map[ast.Expr]bool{}
	ast.Inspect(file, func(node ast.Node) bool {
		if call, ok := node.(*ast.CallExpr); ok {
			fun := call.Fun
			for {
				paren, ok := fun.(*ast.ParenExpr)
				if !ok {
					break
				}
				fun = paren.X
			}
			directCalls[fun] = true
		}
		return true
	})
	for _, decl := range file.Decls {
		owner := ""
		if fn, ok := decl.(*ast.FuncDecl); ok && isHarnessPartitionTest(fn, testingAliases) {
			owner = fn.Name.Name
		}
		ast.Inspect(decl, func(node ast.Node) bool {
			// UnitTest sets this field internally. Explicit uses could make a live
			// harness run without TF_ACC, even under a correctly named TestAcc.
			if ident, ok := node.(*ast.Ident); ok && ident.Name == "IsUnitTest" {
				report(ident, "explicit IsUnitTest is forbidden; use resource.UnitTest in a non-TestAcc test")
			}
			selector, ok := node.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkg, ok := selector.X.(*ast.Ident)
			if !ok || !resourceAliases[pkg.Name] {
				return true
			}
			switch selector.Sel.Name {
			case "Test", "ParallelTest", "UnitTest":
			default:
				return true
			}
			if !directCalls[selector] {
				report(selector, "Terraform harness functions must be called directly, not used as function values")
			} else if owner == "" {
				report(selector, "Terraform harness calls must belong to a top-level Go test, not a helper or initializer")
			} else if selector.Sel.Name == "UnitTest" {
				if strings.HasPrefix(owner, "TestAcc") {
					report(selector, "resource.UnitTest must belong to a non-TestAcc test")
				}
			} else if !strings.HasPrefix(owner, "TestAcc") {
				report(selector, "resource.Test and resource.ParallelTest must belong to a TestAcc test")
			}
			return true
		})
	}
	return violations
}

func isHarnessPartitionTest(fn *ast.FuncDecl, testingAliases map[string]bool) bool {
	name := fn.Name.Name
	if fn.Recv != nil || !strings.HasPrefix(name, "Test") {
		return false
	}
	if suffix := strings.TrimPrefix(name, "Test"); suffix != "" {
		r, _ := utf8.DecodeRuneInString(suffix)
		if unicode.IsLower(r) {
			return false
		}
	}
	if fn.Type.TypeParams != nil || fn.Type.Results != nil || len(fn.Type.Params.List) != 1 {
		return false
	}
	param := fn.Type.Params.List[0]
	if len(param.Names) > 1 {
		return false
	}
	ptr, ok := param.Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	typ, ok := ptr.X.(*ast.SelectorExpr)
	if !ok || typ.Sel.Name != "T" {
		return false
	}
	pkg, ok := typ.X.(*ast.Ident)
	return ok && testingAliases[pkg.Name]
}

func TestHarnessPartitionSynthetic(t *testing.T) {
	cases := []struct {
		name   string
		source string
		want   string
	}{
		{"live", `func TestAccLive(t *testing.T) { resource.Test(t, resource.TestCase{}) }`, ""},
		{"parallel", `func TestAccLive(t *testing.T) { resource.ParallelTest(t, resource.TestCase{}) }`, ""},
		{"unit", `func TestOffline(t *testing.T) { resource.UnitTest(t, resource.TestCase{}) }`, ""},
		{"alias live", `func TestAccLive(t *testing.T) { tfresource.Test(t, tfresource.TestCase{}) }`, ""},
		{"alias unit", `func TestOffline(t *testing.T) { tfresource.UnitTest(t, tfresource.TestCase{}) }`, ""},
		{"alias wrong live", `func TestOffline(t *testing.T) { tfresource.Test(t, tfresource.TestCase{}) }`, "must belong to a TestAcc"},
		{"alias wrong unit", `func TestAccLive(t *testing.T) { tfresource.UnitTest(t, tfresource.TestCase{}) }`, "must belong to a non-TestAcc"},
		{"nested live", `func TestAccLive(t *testing.T) { t.Run("offline-looking", func(t *testing.T) { resource.Test(t, resource.TestCase{}) }) }`, ""},
		{"nested unit", `func TestOffline(t *testing.T) { t.Run("TestAcc", func(t *testing.T) { resource.UnitTest(t, resource.TestCase{}) }) }`, ""},
		{"nested wrong", `func TestOffline(t *testing.T) { t.Run("TestAcc", func(t *testing.T) { resource.ParallelTest(t, resource.TestCase{}) }) }`, "must belong to a TestAcc"},
		{"mixed offline", `func TestOffline(t *testing.T) { resource.UnitTest(t, resource.TestCase{}); resource.Test(t, resource.TestCase{}) }`, "must belong to a TestAcc"},
		{"mixed acceptance", `func TestAccLive(t *testing.T) { resource.Test(t, resource.TestCase{}); resource.UnitTest(t, resource.TestCase{}) }`, "must belong to a non-TestAcc"},
		{"helper live", `func runLive(t *testing.T) { resource.Test(t, resource.TestCase{}) }`, "not a helper"},
		{"helper unit", `func runUnit(t *testing.T) { resource.UnitTest(t, resource.TestCase{}) }`, "not a helper"},
		{"misleading helper", `func TestaccHelper(t *testing.T) { resource.Test(t, resource.TestCase{}) }`, "not a helper"},
		{"wrong signature", `func TestAccHelper() { resource.Test(nil, resource.TestCase{}) }`, "not a helper"},
		{"method", `func (s *suite) TestAccHelper(t *testing.T) { resource.Test(t, resource.TestCase{}) }`, "not a helper"},
		{"initializer", `var run = func() { resource.Test(nil, resource.TestCase{}) }`, "not a helper"},
		{"function value", `func TestAccLive(t *testing.T) { run := resource.Test; run(t, resource.TestCase{}) }`, "function values"},
		{"package function value", `var run = resource.UnitTest`, "function values"},
		{"parenthesized call", `func TestAccLive(t *testing.T) { (resource.Test)(t, resource.TestCase{}) }`, ""},
		{"explicit true", `func TestAccLive(t *testing.T) { resource.Test(t, resource.TestCase{IsUnitTest: true}) }`, "explicit IsUnitTest"},
		{"explicit false", `func TestAccLive(t *testing.T) { resource.Test(t, resource.TestCase{IsUnitTest: false}) }`, "explicit IsUnitTest"},
		{"explicit assignment", `func configure() { c := resource.TestCase{}; c.IsUnitTest = true }`, "explicit IsUnitTest"},
		{"ordinary check helper", `func check() { resource.TestCheckResourceAttr("a", "b", "c") }`, ""},
		{"synthetic text", "var text = `resource.Test(t, resource.TestCase{IsUnitTest: true})`", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			alias := "resource"
			if strings.HasPrefix(tc.name, "alias ") {
				alias = "tfresource"
			}
			source := "package example\nimport \"testing\"\nimport " + alias + " \"github.com/hashicorp/terraform-plugin-testing/helper/resource\"\n" + tc.source
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "example_test.go", source, 0)
			if err != nil {
				t.Fatal(err)
			}
			got := harnessPartitionViolations(fset, file)
			if tc.want == "" {
				if len(got) != 0 {
					t.Fatalf("unexpected violations: %v", got)
				}
			} else if len(got) != 1 || !strings.Contains(got[0], tc.want) {
				t.Fatalf("violations = %v, want one containing %q", got, tc.want)
			}
		})
	}
}
