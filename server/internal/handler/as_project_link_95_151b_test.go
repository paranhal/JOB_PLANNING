package handler

import (
	"path/filepath"
	"strings"
	"testing"
)

// 조치 화면의 사업 후보 목록은 연결 상태가 아니다. 아무것도 고르지 않았으면 「연결된 사업이 없습니다」를
// 분명히 알려야 한다(R2611-016: 후보 1건이 연결된 것처럼 보여 미연결 건을 놓침).
func Test95_151B_SiteProjectNoteSaysNotLinked(t *testing.T) {
	js := string(mustRead(t, filepath.Join(findStaticRoot(t), "js", "project_picker.js")))
	if !strings.Contains(js, "이 사이트가 등록된 사업 ' + n + '건'") {
		t.Fatal("후보 건수 문구가 없다")
	}
	if !strings.Contains(js, "if (!this.selectedId) note +=") ||
		!strings.Contains(js, "연결된 사업이 없습니다. 하나를 눌러 선택한 뒤 저장하세요") {
		t.Fatal("선택이 없을 때 「연결된 사업이 없습니다」 안내가 없다")
	}
}
