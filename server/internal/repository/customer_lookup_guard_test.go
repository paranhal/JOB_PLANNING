package repository

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCustomerLookupSQLHasNoOrgOrTopOnly(t *testing.T) {
	src, err := os.ReadFile("customer_lookup.go")
	if err != nil {
		src, err = os.ReadFile("internal/repository/customer_lookup.go")
	}
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	if strings.Contains(body, "org_id") {
		t.Fatal("customer_lookup.go 에 org_id 조건이 있다")
	}
	if strings.Contains(body, "has_parent=0") || strings.Contains(body, "has_parent = 0") {
		t.Fatal("customer_lookup.go 에 최상위만 조건이 있다")
	}
}

func TestNoCustomerPickerSQLOutsideLookup(t *testing.T) {
	allowFn := map[string]string{
		"List":               "고객현황 목록",
		"ListForAPI":         "영업관리 연동 API",
		"SearchCustomersWithoutConfig": "Lookup 어댑터",
		"SearchWorkTaskCustomers":      "Lookup 어댑터",
		"Lookup":             "유일한 구현",
		"LookupByIDs":        "유일한 구현",
		"listFromLookup":     "Lookup 래퍼",
		"ListSelectable":     "Lookup 래퍼",
		"ListParties":        "Lookup 래퍼",
		"ListAll":            "백업·현황 허용 래퍼",
		"ListAllInOrg":       "deprecated 래퍼",
		"ListForReceipt":     "Lookup 래퍼",
		"ListForReceiptIncluding": "Lookup 래퍼",
	}
	root := "."
	if _, err := os.Stat("customer_lookup.go"); err != nil {
		root = "internal/repository"
	}
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
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
			if !strings.Contains(low, "customer") && name != "ListAll" && name != "ListAllInOrg" && name != "Lookup" {
				continue
			}
			if strings.HasPrefix(low, "apply") || strings.HasPrefix(low, "migrate") || strings.HasPrefix(low, "ensure") || strings.HasPrefix(low, "find") || strings.HasPrefix(low, "seed") {
				continue
			}
			start := fset.Position(fn.Pos()).Offset
			end := fset.Position(fn.End()).Offset
			if start < 0 || end > len(src) || start >= end {
				continue
			}
			body := strings.ToUpper(string(src[start:end]))
			if !strings.Contains(body, "FROM CUSTOMERS") {
				continue
			}
			if !strings.Contains(body, " LIKE ") && !strings.Contains(low, "listall") && !strings.Contains(low, "listfor") && name != "Lookup" {
				continue
			}
			if allowFn[name] != "" {
				continue
			}
			t.Errorf("%s:%s FROM customers 선택 SQL — CustomerLookup 만 허용", filepath.Base(path), name)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestCustomerLookupGuardFailsOnOrgFilterSample(t *testing.T) {
	sample := `func (r *CustomerRepo) Lookup(q CustomerQuery) { _ = "WHERE c.org_id=? AND has_parent=0" }`
	if !strings.Contains(sample, "org_id") || !strings.Contains(sample, "has_parent=0") {
		t.Fatal("양성 샘플이 규칙을 안 건드린다")
	}
	src, _ := os.ReadFile("customer_lookup.go")
	if src == nil {
		src, _ = os.ReadFile("internal/repository/customer_lookup.go")
	}
	if strings.Contains(string(src), "org_id") || strings.Contains(string(src), "has_parent=0") {
		t.Fatal("실제 Lookup 이 양성 샘플과 같이 org/최상위 조건을 가졌다")
	}
}
