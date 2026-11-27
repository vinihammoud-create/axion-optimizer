package diag

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
	"testing"
)

// Every rule is classified by hand: each finding ID written in the rules
// (a literal, or a literal prefix plus a device) has an entry in certRules,
// so no rule falls to the default without someone having read it.
func TestEveryRuleIDIsClassified(t *testing.T) {
	ents, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	seen := 0
	for _, e := range ents {
		n := e.Name()
		if !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, n, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			lit, ok := node.(*ast.CompositeLit)
			if !ok {
				return true
			}
			if id, ok := lit.Type.(*ast.Ident); !ok || id.Name != "Finding" {
				return true
			}
			for _, el := range lit.Elts {
				kv, ok := el.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				if k, ok := kv.Key.(*ast.Ident); !ok || k.Name != "ID" {
					continue
				}
				key := ""
				switch v := kv.Value.(type) {
				case *ast.BasicLit:
					key, _ = strconv.Unquote(v.Value)
				case *ast.BinaryExpr:
					if b, ok := v.X.(*ast.BasicLit); ok && v.Op == token.ADD {
						p, _ := strconv.Unquote(b.Value)
						key = p + "*"
					}
				}
				if key == "" {
					continue // an ID held in a variable (gpu.nvdrs.frl.<exe>)
				}
				seen++
				if _, ok := ruleCertainty(key, ""); !ok {
					t.Errorf("%s: a regra %q não tem certeza classificada em certRules", fset.Position(kv.Pos()), key)
				}
			}
			return true
		})
	}
	if seen < 150 {
		t.Fatalf("só %d IDs encontrados nas regras: a leitura do código falhou", seen)
	}
}

func TestCertRulesValues(t *testing.T) {
	sevs := map[string]bool{SevCritical: true, SevWarning: true, SevInfo: true}
	for k, c := range certRules {
		if !validCertainty(c) {
			t.Errorf("%s: certeza inválida %q", k, c)
		}
		if i := strings.LastIndexByte(k, ':'); i >= 0 && !sevs[k[i+1:]] {
			t.Errorf("%s: severidade desconhecida", k)
		}
	}
}

func TestCertaintyOf(t *testing.T) {
	for _, c := range []struct {
		x    Finding
		want string
	}{
		// A direct reading that states the problem.
		{Finding{ID: "game.mode", Severity: SevWarning}, CertConfirmed},
		{Finding{ID: "disk.health.1", Severity: SevCritical}, CertConfirmed},
		{Finding{ID: "display.refresh.\\\\.\\DISPLAY1", Severity: SevCritical}, CertConfirmed},
		{Finding{ID: "ram.xmp", Severity: SevWarning}, CertConfirmed},
		// A criterion of the Axion, a hint or a choice of the user.
		{Finding{ID: "ram.low", Severity: SevWarning}, CertPossible},
		{Finding{ID: "ram.xmp", Severity: SevInfo}, CertPossible},
		{Finding{ID: "gpu.driver.old", Severity: SevWarning}, CertPossible},
		{Finding{ID: "display.refresh.\\\\.\\DISPLAY1", Severity: SevInfo}, CertPossible},
		{Finding{ID: "gpu.nvdrs.frl.cs2.exe", Severity: SevWarning}, CertPossible},
		// What could not be read.
		{Finding{ID: "disk.health.unknown.0", Severity: SevInfo}, CertInsufficient},
		// Unknown rule: never overclaims.
		{Finding{ID: "regra.nova", Severity: SevCritical}, CertPossible},
		// The rule's own value wins over the table...
		{Finding{ID: "therm.now", Severity: SevWarning, Certainty: CertPossible}, CertPossible},
		// ...but a problem is never "normal", and good is always normal.
		{Finding{ID: "game.mode", Severity: SevWarning, Certainty: CertNormal}, CertConfirmed},
		{Finding{ID: "game.mode", Severity: SevGood, Certainty: CertConfirmed}, CertNormal},
		{Finding{ID: "regra.nova", Severity: SevGood}, CertNormal},
	} {
		if got := certaintyOf(c.x); got != c.want {
			t.Errorf("%s (%s, %q): %q, esperado %q", c.x.ID, c.x.Severity, c.x.Certainty, got, c.want)
		}
	}
}
