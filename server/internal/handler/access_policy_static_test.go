package handler

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func Test95_99_T5_DataScopeIsCanonical(t *testing.T) {
	_, thisFile, _, _ := runtime.Caller(0)
	dir := filepath.Dir(thisFile)
	src, err := os.ReadFile(filepath.Join(dir, "access_policy.go"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	if !strings.Contains(body, "func DataScopeFor(") || !strings.Contains(body, "func OrgScopeIDs(") {
		t.Fatal("DataScopeFor / OrgScopeIDs 없음")
	}
	if !strings.Contains(body, "currentOrg(c)") || !strings.Contains(body, "isReadOnly(c)") {
		t.Fatal("기존 함수를 내부에서 써야 한다")
	}
	root := dir
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		if filepath.Base(path) == "access_policy.go" {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		txt := string(b)
		if strings.Contains(txt, "func DataScopeFor(") {
			t.Errorf("%s 에 DataScopeFor 를 또 정의했다", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func Test95_99_T9_HandlerUsesDataScope(t *testing.T) {
	_, thisFile, _, _ := runtime.Caller(0)
	dir := filepath.Dir(thisFile)
	asSrc, err := os.ReadFile(filepath.Join(dir, "as.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(asSrc), "func (h *ASHandler) asInScope(") || !strings.Contains(string(asSrc), "func (h *ASHandler) asWritable(") {
		t.Fatal("AS 단건이 asInScope/asWritable 가 아니다")
	}
	execSrc, err := os.ReadFile(filepath.Join(dir, "exec.go"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(execSrc)
	if !strings.Contains(body, "cLevelWriteBlocked(method, path)") {
		t.Fatal("GuardCLevelHidden 이 cLevelWriteBlocked 를 쓰지 않는다")
	}
	if strings.Contains(body, "cLevelHiddenPath(path)") {
		t.Fatal("GET 가드가 다시 cLevelHiddenPath 이다")
	}
}
