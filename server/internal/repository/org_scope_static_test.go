package repository

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var orgScopeTokenRe = regexp.MustCompile(`org_id|appendOrg|mustOrgSQL|VisibleInOrg`)

func TestOrgScopeStaticAudit(t *testing.T) {
	reason := orgScopeAllowlist["SearchWorkTaskCustomers"]
	if !strings.Contains(reason, "95-9A") {
		t.Fatal("SearchWorkTaskCustomers 를 orgScopeAllowlist 에 95-9A 사유와 함께 등록해야 한다")
	}
	if !strings.Contains(orgScopeAllowlist["SearchCustomersWithoutConfig"], "Q1") {
		t.Fatal("SearchCustomersWithoutConfig 를 orgScopeAllowlist 에 95-9A Q1 사유와 함께 등록해야 한다")
	}
	if !strings.Contains(orgScopeAllowlist["lookupCustomerName"], "95-2E") {
		t.Fatal("lookupCustomerName 를 orgScopeAllowlist 에 전사 공용 마스터 — 95-2E 사유와 함께 등록해야 한다")
	}
	if !strings.Contains(orgScopeAllowlist["List"], "95-2E") {
		t.Fatal("CustomerRepo.List 를 orgScopeAllowlist 에 전사 공용 마스터 — 95-2E 사유와 함께 등록해야 한다")
	}
	if !strings.Contains(orgScopeAllowlist["ListWorkPickProjects"], "95-9A") {
		t.Fatal("ListWorkPickProjects 를 orgScopeAllowlist 에 95-9A 사유와 함께 등록해야 한다")
	}
	if !strings.Contains(orgScopeAllowlist["listProjects"], "95-2E") {
		t.Fatal("listProjects 전 직원 조회는 allowlist 에 95-2E 사유를 적는다. 쓰기는 GetVisible(write)")
	}

	root := "."
	if _, err := os.Stat("work_task_sites.go"); err != nil {
		root = "internal/repository"
	}
	fset := token.NewFileSet()
	var missing []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		base := filepath.Base(path)
		if !strings.HasSuffix(base, ".go") || strings.HasSuffix(base, "_test.go") {
			return nil
		}
		if strings.Contains(base, "_schema.go") || base == "org_scope_static.go" {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		f, err := parser.ParseFile(fset, path, src, 0)
		if err != nil {
			return err
		}
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || fn.Name == nil {
				continue
			}
			name := fn.Name.Name
			low := strings.ToLower(name)
			if strings.HasPrefix(low, "apply") || strings.HasPrefix(low, "migrate") ||
				strings.HasPrefix(low, "backfill") || strings.HasPrefix(low, "ensure") ||
				strings.HasPrefix(low, "seed") || strings.HasPrefix(low, "find") {
				continue
			}
			if !strings.Contains(low, "search") && !strings.Contains(low, "lookup") &&
				name != "SearchWorkTaskCustomers" && name != "ListWorkPickProjects" && name != "listProjects" {
				continue
			}
			start := fset.Position(fn.Pos()).Offset
			end := fset.Position(fn.End()).Offset
			if start < 0 || end > len(src) || start >= end {
				continue
			}
			body := string(src[start:end])
			if !orgScopeIsCustomerSearch(body) {
				continue
			}
			if orgScopeTokenRe.MatchString(body) {
				continue
			}
			if orgScopeAllowlist[name] != "" {
				continue
			}
			missing = append(missing, base+":"+name)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(missing) > 0 {
		t.Fatalf("조직 조건 없는 조회 (allowlist 필요):\n%s", strings.Join(missing, "\n"))
	}
}

func orgScopeIsCustomerSearch(body string) bool {
	up := strings.ToUpper(body)
	if !strings.Contains(up, "FROM CUSTOMERS") && !strings.Contains(up, "JOIN CUSTOMERS") {
		return false
	}
	return strings.Contains(up, " LIKE ")
}
