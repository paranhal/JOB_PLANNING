package model

import "testing"

func TestAttachmentDisplayName(t *testing.T) {
	cases := []struct {
		name, fileName, filePath, want string
	}{
		{"원본명", "조치완료보고서.pdf", "data/uploads/as/x/1_조치완료보고서.pdf", "조치완료보고서.pdf"},
		{"구저장 접두사", "1234567890123_현장사진.jpg", "", "현장사진.jpg"},
		{"파일명 비어 경로만", "", "data/uploads/as/x/1234567890123456_결과.docx", "결과.docx"},
		{"자산 이미지", "AEZ-200E23-001_img_1.jpg", "", "AEZ-200E23-001_img_1.jpg"},
		{"짧은 숫자 접두사", "12_memo.txt", "", "12_memo.txt"},
	}
	for _, tc := range cases {
		got := Attachment{FileName: tc.fileName, FilePath: tc.filePath}.DisplayName()
		if got != tc.want {
			t.Errorf("%s: got %q want %q", tc.name, got, tc.want)
		}
	}
}
