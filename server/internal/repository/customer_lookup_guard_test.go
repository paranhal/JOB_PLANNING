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

func lookupSQLHasOrgFilter(sql string) bool {
	s := strings.ToLower(sql)
	return strings.Contains(s, "c.org_id=") || strings.Contains(s, "c.org_id =")
}

func lookupSQLHasTopOnly(sql string) bool {
	s := strings.ReplaceAll(strings.ToLower(sql), " ", "")
	return strings.Contains(s, "has_parent=0")
}

func TestCustomerLookupSQLHasNoOrgOrTopOnly(t *testing.T) {
	r := &CustomerRepo{}
	def, _ := r.lookupWhere(CustomerQuery{Scope: ScopeSite})
	if lookupSQLHasOrgFilter(def) {
		t.Fatalf("기본 호출 SQL에 c.org_id= 가 있다: %s", def)
	}
	if lookupSQLHasTopOnly(def) {
		t.Fatalf("기본 호출 SQL에 has_parent=0 이 있다: %s", def)
	}

	all, _ := r.lookupWhere(CustomerQuery{Scope: ScopeSite, OrgID: OrgAll})
	if lookupSQLHasOrgFilter(all) {
		t.Fatalf("OrgAll SQL에 c.org_id= 가 있다: %s", all)
	}
	if lookupSQLHasTopOnly(all) {
		t.Fatalf("OrgAll SQL에 has_parent=0 이 있다: %s", all)
	}

	org, _ := r.lookupWhere(CustomerQuery{Scope: ScopeSite, OrgID: "O01"})
	if !lookupSQLHasOrgFilter(org) {
		t.Fatalf("OrgID 지정 SQL에 c.org_id=? 가 없다: %s", org)
	}
	if lookupSQLHasTopOnly(org) {
		t.Fatalf("OrgID 경로에 has_parent=0 이 있다: %s", org)
	}
}

func TestNoCustomerPickerSQLOutsideLookup(t *testing.T) {
	allowFn := map[string]string{
		"List":                         "고객현황 목록",
		"ListForAPI":                   "영업관리 연동 API",
		"SearchCustomersWithoutConfig": "Lookup 어댑터",
		"SearchWorkTaskCustomers":      "Lookup 어댑터",
		"Lookup":                       "유일한 구현",
		"LookupByIDs":                  "유일한 구현",
		"listFromLookup":               "Lookup 래퍼",
		"ListSelectable":               "Lookup 래퍼",
		"ListParties":                  "Lookup 래퍼",
		"ListAll":                      "백업·현황 허용 래퍼",
		"ListAllInOrg":                 "deprecated 래퍼",
		"ListForReceipt":               "Lookup 래퍼",
		"ListForReceiptIncluding":      "Lookup 래퍼",
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
	if !lookupSQLHasOrgFilter(sample) || !lookupSQLHasTopOnly(sample) {
		t.Fatal("양성 샘플이 규칙을 안 건드린다")
	}
	if !lookupSQLHasOrgFilter(sample) {
		t.Fatal("가드가 기본 경로 조직 조건을 놓쳤다")
	}
	r := &CustomerRepo{}
	got, _ := r.lookupWhere(CustomerQuery{Scope: ScopeSite})
	if lookupSQLHasOrgFilter(got) || lookupSQLHasTopOnly(got) {
		t.Fatal("실제 기본 Lookup SQL이 양성 샘플과 같이 조직/최상위 조건을 가졌다")
	}
}
