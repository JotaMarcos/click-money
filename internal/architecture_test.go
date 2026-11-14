// Teste de arquitetura: verifica as regras de dependência descritas em
// docs/architecture/estrutura-de-diretorios.md (ADR 0001). Ele lê apenas os
// imports dos arquivos de produção, então roda sem compilar nada.
package internal_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const modulePath = "github.com/JotaMarcos/click-money"

// Pacotes de platform que cada camada interna pode importar.
var (
	platformForDomain = []string{"kernel"}
	platformForApp    = []string{"kernel", "clock", "idgen"}
)

// checkImport devolve o motivo pelo qual o pacote pkg (caminho relativo à raiz
// do módulo, como "internal/ledger/domain") não pode importar imp (caminho de
// import completo). Devolve "" quando o import é permitido.
func checkImport(pkg, imp string) string {
	from := strings.Split(pkg, "/")
	if len(from) < 2 || from[0] != "internal" {
		return ""
	}
	module, layer := from[1], ""
	if len(from) > 2 {
		layer = from[2]
	}

	if !strings.HasPrefix(imp, modulePath+"/") {
		if isStdlib(imp) || module == "platform" {
			return ""
		}
		if layer == "domain" || layer == "app" {
			return layer + " só pode usar a biblioteca padrão, não dependência externa"
		}
		return ""
	}

	to := strings.Split(strings.TrimPrefix(imp, modulePath+"/"), "/")
	if to[0] == "cmd" {
		return "código em internal não importa cmd"
	}
	if to[0] != "internal" || len(to) < 2 {
		// Pacotes de apoio na raiz (como migrations) só interessam à platform.
		if module != "platform" && (layer == "domain" || layer == "app") {
			return layer + " não importa " + to[0]
		}
		return ""
	}
	toModule, toLayer, toPkg := to[1], "", ""
	if len(to) > 2 {
		toLayer, toPkg = to[2], to[2]
	}

	switch {
	case module == "platform":
		if toModule != "platform" {
			return "platform não conhece módulos de negócio"
		}
	case toModule == "platform":
		if layer == "domain" && !contains(platformForDomain, toPkg) {
			return "domain só pode importar platform/kernel"
		}
		if layer == "app" && !contains(platformForApp, toPkg) {
			return "app só pode importar platform/kernel, platform/clock e platform/idgen"
		}
	case toModule == module:
		if layer == "domain" && toLayer != "domain" {
			return "domain não importa " + toLayer + " do próprio módulo"
		}
		if layer == "app" && toLayer != "domain" && toLayer != "app" {
			return "app não importa " + toLayer + " do próprio módulo"
		}
	default:
		if layer == "domain" {
			return "domain não importa outro módulo"
		}
		if toLayer != "app" {
			return "um módulo só fala com outro pelo pacote app dele"
		}
	}
	return ""
}

// isStdlib usa a convenção do Go: pacotes da biblioteca padrão não têm ponto
// no primeiro elemento do caminho.
func isStdlib(imp string) bool {
	first, _, _ := strings.Cut(imp, "/")
	return !strings.Contains(first, ".")
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func TestModulePathMatchesGoMod(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "go.mod"))
	if err != nil {
		t.Fatalf("lendo go.mod: %v", err)
	}
	first, _, _ := strings.Cut(string(data), "\n")
	if got := strings.TrimSpace(strings.TrimPrefix(first, "module ")); got != modulePath {
		t.Fatalf("go.mod declara %q, o teste de arquitetura espera %q", got, modulePath)
	}
}

func TestDependencyRules(t *testing.T) {
	fset := token.NewFileSet()
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		pkg := "internal"
		if dir := filepath.ToSlash(filepath.Dir(path)); dir != "." {
			pkg += "/" + dir
		}
		for _, spec := range file.Imports {
			imp, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return err
			}
			if reason := checkImport(pkg, imp); reason != "" {
				t.Errorf("%s importa %s: %s", filepath.ToSlash(path), imp, reason)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("percorrendo internal: %v", err)
	}
}

// Ainda não há módulos de negócio, então as regras são exercitadas aqui com
// caminhos hipotéticos: sem isto o teste acima passaria sem provar nada.
func TestCheckImport(t *testing.T) {
	const m = modulePath + "/internal/"
	cases := []struct {
		name    string
		pkg     string
		imp     string
		allowed bool
	}{
		{"domain usa biblioteca padrão", "internal/ledger/domain", "errors", true},
		{"domain usa kernel", "internal/ledger/domain", m + "platform/kernel", true},
		{"domain usa subpacote do próprio domain", "internal/ledger/domain", m + "ledger/domain/fifo", true},
		{"domain usa banco", "internal/ledger/domain", m + "platform/db", false},
		{"domain usa clock", "internal/ledger/domain", m + "platform/clock", false},
		{"domain usa app", "internal/ledger/domain", m + "ledger/app", false},
		{"domain usa adapters", "internal/ledger/domain", m + "ledger/adapters/postgres", false},
		{"domain usa outro módulo", "internal/ledger/domain", m + "customers/app", false},
		{"domain usa dependência externa", "internal/ledger/domain", "github.com/jackc/pgx/v5", false},

		{"app usa o próprio domain", "internal/ledger/app", m + "ledger/domain", true},
		{"app usa clock", "internal/ledger/app", m + "platform/clock", true},
		{"app usa idgen", "internal/ledger/app", m + "platform/idgen", true},
		{"app usa app de outro módulo", "internal/ledger/app", m + "customers/app", true},
		{"app usa domain de outro módulo", "internal/ledger/app", m + "customers/domain", false},
		{"app usa adapters", "internal/ledger/app", m + "ledger/adapters/http", false},
		{"app usa httpx", "internal/ledger/app", m + "platform/httpx", false},
		{"app usa dependência externa", "internal/ledger/app", "github.com/jackc/pgx/v5", false},

		{"adapter usa app e platform", "internal/ledger/adapters/postgres", m + "platform/db", true},
		{"adapter usa o próprio domain", "internal/ledger/adapters/http", m + "ledger/domain", true},
		{"adapter usa dependência externa", "internal/ledger/adapters/postgres", "github.com/jackc/pgx/v5", true},
		{"adapter usa app de outro módulo", "internal/ledger/adapters/http", m + "customers/app", true},
		{"adapter usa adapter de outro módulo", "internal/ledger/adapters/http", m + "customers/adapters/postgres", false},

		{"platform usa platform", "internal/platform/outbox", m + "platform/db", true},
		{"platform usa dependência externa", "internal/platform/db", "github.com/jackc/pgx/v5", true},
		{"platform usa módulo de negócio", "internal/platform/httpx", m + "ledger/app", false},

		{"internal usa cmd", "internal/ledger/adapters/http", modulePath + "/cmd/api", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reason := checkImport(tc.pkg, tc.imp)
			if tc.allowed && reason != "" {
				t.Errorf("deveria ser permitido, mas foi recusado: %s", reason)
			}
			if !tc.allowed && reason == "" {
				t.Error("deveria ser recusado, mas foi permitido")
			}
		})
	}
}
