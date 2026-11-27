package handler

import (
	"encoding/json"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

// 95-224 §3: 1회 등록 요청 — 정답 비번 → 권한, 오답 → 거부+기록, 5회 실패 → 잠금, 1건 등록 후 소멸,
// 30분 만료, 다른 사업 재사용 불가, 비밀번호 미노출.
func TestHTTP95_224_SiteAddOnce(t *testing.T) {
	v := setup196(t)
	e := v.e
	_ = repository.NewSettingsRepo(v.db).EnsureASCompletedEditPassword(HashPassword)
	tk := cookie183(t, "tk224", "tech", "O01", false, model.PermWorkboard)
	add := func(pid string, ck *http.Cookie) int {
		return v.do(t, "POST", "/projects/"+pid+"/sites/add-schools", url.Values{"customer_id": {"C196-L1"}}, ck, true).Code
	}
	unlock := func(pid, pw string, ck *http.Cookie) (int, string) {
		r := v.do(t, "POST", "/projects/"+pid+"/sites/unlock", url.Values{"unlock_password": {pw}}, ck, true)
		return r.Code, r.Body.String()
	}
	// 권한 없는 조직원: 사이트 추가 403
	if c := add("P196-LIB", tk); c != http.StatusForbidden {
		t.Fatalf("권한 없는 등록 %d want 403", c)
	}
	// 화면에 1회 등록 요청 폼이 보인다
	if b := v.do(t, "GET", "/projects/P196-LIB/sites", nil, tk, false).Body.String(); !strings.Contains(b, "data-site-once-request") {
		t.Fatal("1회 등록 요청 폼이 안 보인다")
	}
	// 오답 → 403, 비밀번호가 응답에 없다
	code, body := unlock("P196-LIB", "wrong-pass-xyz", tk)
	if code != http.StatusForbidden || strings.Contains(body, "wrong-pass-xyz") {
		t.Fatalf("오답 %d body=%s", code, body)
	}
	// 정답 → 권한 부여, 이후 등록 200
	code, body = unlock("P196-LIB", repository.DefaultASCompletedEditPassword, tk)
	if code != http.StatusOK || strings.Contains(body, repository.DefaultASCompletedEditPassword) {
		t.Fatalf("정답 %d body=%s", code, body)
	}
	// 다른 사업에는 재사용 불가
	if c := add("P196-SCH", tk); c != http.StatusForbidden {
		t.Fatalf("다른 사업 재사용 %d want 403", c)
	}
	if c := add("P196-LIB", tk); c != http.StatusOK {
		t.Fatalf("권한 후 등록 %d want 200", c)
	}
	// 변경 이력: 「1회 등록 요청으로 등록」 메모, 비밀번호는 어디에도 없다
	var n int
	_ = v.db.QueryRow(`SELECT COUNT(*) FROM data_change_logs WHERE table_name='project_sites' AND entity_id='P196-LIB' AND reason=?`, repository.SiteAddReason).Scan(&n)
	if n != 1 {
		t.Fatalf("1회 등록 이력 %d건 want 1", n)
	}
	var leak int
	_ = v.db.QueryRow(`SELECT COUNT(*) FROM data_change_logs WHERE after_json LIKE ? OR summary LIKE ? OR reason LIKE ?`, "%"+repository.DefaultASCompletedEditPassword+"%", "%"+repository.DefaultASCompletedEditPassword+"%", "%"+repository.DefaultASCompletedEditPassword+"%").Scan(&leak)
	if leak != 0 {
		t.Fatal("변경 이력에 비밀번호가 남았다")
	}
	// 1건 등록 후 소멸 → 다시 403
	if c := add("P196-LIB", tk); c != http.StatusForbidden {
		t.Fatalf("소멸 후 등록 %d want 403", c)
	}
	_ = e
}

func TestHTTP95_224_SiteAddOnceLockout(t *testing.T) {
	v := setup196(t)
	_ = repository.NewSettingsRepo(v.db).EnsureASCompletedEditPassword(HashPassword)
	tk := cookie183(t, "lock224", "tech", "O01", false, model.PermWorkboard)
	unlock := func(pw string) (int, string) {
		r := v.do(t, "POST", "/projects/P196-LIB/sites/unlock", url.Values{"unlock_password": {pw}}, tk, true)
		return r.Code, r.Body.String()
	}
	for i := 0; i < repository.SiteAddMaxFails; i++ {
		if c, _ := unlock("bad"); c != http.StatusForbidden {
			t.Fatalf("%d회차 오답 %d", i+1, c)
		}
	}
	// 6번째는 정답이어도 잠금(429)
	if c, b := unlock(repository.DefaultASCompletedEditPassword); c != http.StatusTooManyRequests {
		t.Fatalf("잠금 중 시도 %d want 429 (%s)", c, b)
	}
}

// 30분 경과한 권한은 쓸 수 없고, 소멸·잠금 계산은 저장소 단위로 확인한다.
func TestSiteAddGrantExpiryAndLock(t *testing.T) {
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "g224.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	r := repository.NewASUnlockRepo(db)
	key := repository.SiteAddKey("P1")
	if _, err := db.Exec(`INSERT INTO as_edit_unlocks(unlock_id,as_id,user_id,unlocked_at,expires_at) VALUES('UL-x',?,?,?,?)`,
		key, "u1", time.Now().Add(-40*time.Minute).Format(time.RFC3339), time.Now().Add(-10*time.Minute).Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	if ok, _, _ := r.HasActive(key, "u1"); ok {
		t.Fatal("만료된 권한이 살아 있다")
	}
	if _, err := r.Grant(key, "u1"); err != nil {
		t.Fatal(err)
	}
	if ok, _, _ := r.HasActive(key, "u1"); !ok {
		t.Fatal("부여한 권한이 없다")
	}
	_ = r.Revoke(key, "u1")
	if ok, _, _ := r.HasActive(key, "u1"); ok {
		t.Fatal("Revoke 뒤에도 권한이 남아 있다")
	}
	for i := 0; i < repository.SiteAddMaxFails; i++ {
		_ = r.LogAttempt(key, "u2", "u2", "127.0.0.1", false)
	}
	if _, locked := r.SiteAddLockedUntil("u2", time.Now()); !locked {
		t.Fatal("5회 실패 후 잠기지 않았다")
	}
	if _, locked := r.SiteAddLockedUntil("u2", time.Now().Add(11*time.Minute)); locked {
		t.Fatal("10분 뒤에도 잠겨 있다")
	}
	_ = r.LogAttempt(key, "u3", "u3", "127.0.0.1", false)
	_ = r.LogAttempt(key, "u3", "u3", "127.0.0.1", true)
	if _, locked := r.SiteAddLockedUntil("u3", time.Now()); locked {
		t.Fatal("성공 이후 실패 1회로 잠겼다")
	}
}

// 간이 팝업 데이터: 일반 조직원은 등록 권한 없음 + 1회 요청 가능, 관리자는 바로 등록 가능. 대상은 고객의 하위 기관뿐.
func TestHTTP95_224_SitesQuick(t *testing.T) {
	v := setup196(t)
	type quick struct {
		SiteTotal      int  `json:"siteTotal"`
		CanRegister    bool `json:"canRegister"`
		CanRequestOnce bool `json:"canRequestOnce"`
		Children       struct {
			Items []struct {
				ID         string `json:"id"`
				Registered bool   `json:"registered"`
			} `json:"items"`
		} `json:"children"`
	}
	get := func(ck *http.Cookie) quick {
		r := v.do(t, "GET", "/projects/P196-LIB/sites/quick", nil, ck, true)
		if r.Code != http.StatusOK {
			t.Fatalf("quick %d %s", r.Code, r.Body.String())
		}
		var q quick
		if err := json.Unmarshal(r.Body.Bytes(), &q); err != nil {
			t.Fatal(err)
		}
		return q
	}
	tk := cookie183(t, "q224", "tech", "O01", false, model.PermWorkboard)
	q := get(tk)
	if q.CanRegister || !q.CanRequestOnce || q.SiteTotal != 0 || len(q.Children.Items) == 0 {
		t.Fatalf("일반 조직원 %+v", q)
	}
	if a := get(v.vision); !a.CanRegister || a.CanRequestOnce {
		t.Fatalf("관리자 %+v", a)
	}
	// 사업 고객 밖(마스터 전체)은 후보에 없다: 하위 기관이 아닌 C196-SCH 계열 학교는 도서관 사업 후보에 없어야 한다.
	for _, it := range q.Children.Items {
		if it.ID == "C196-S1" || it.ID == "C196-S2" {
			t.Fatalf("다른 고객의 학교 %s 가 후보에 있다", it.ID)
		}
	}
}
