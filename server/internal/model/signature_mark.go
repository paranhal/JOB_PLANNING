package model

import (
	"bytes"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"strings"
	"unicode/utf8"
)

// SignatureMark 서명 칸에 넣을 그림 또는 성명. D-94
type SignatureMark struct {
	PNG  []byte
	Name string
}

// PersonDisplayName 성명이 없으면 아이디(로그인명), 그것도 없으면 사용자 ID.
func PersonDisplayName(fullName, username, userID string) string {
	if s := strings.TrimSpace(fullName); s != "" {
		return s
	}
	if s := strings.TrimSpace(username); s != "" {
		return s
	}
	return strings.TrimSpace(userID)
}

// UsableSignaturePNG 등록·파일이 있고 이미지로 해석 가능할 때만 참.
func UsableSignaturePNG(png []byte) bool {
	if len(png) == 0 {
		return false
	}
	_, _, err := image.DecodeConfig(bytes.NewReader(png))
	return err == nil
}

// SignatureOrName 서명 칸 판정. 그림이 없거나 읽을 수 없으면 이름.
func SignatureOrName(png []byte, fullName, username, userID string) SignatureMark {
	name := PersonDisplayName(fullName, username, userID)
	if UsableSignaturePNG(png) {
		return SignatureMark{PNG: png, Name: name}
	}
	return SignatureMark{Name: name}
}

// SignatureOrNameUser 계정과 읽은 PNG로 판정.
func SignatureOrNameUser(u *User, png []byte) SignatureMark {
	if u == nil {
		return SignatureOrName(png, "", "", "")
	}
	return SignatureOrName(png, u.FullName, u.Username, u.UserID)
}

// StampNameClass 화면 서명 칸 글자 크기. 길면 축소(말줄임 없음).
func StampNameClass(name string) string {
	n := utf8.RuneCountInString(strings.TrimSpace(name))
	switch {
	case n >= 6:
		return "text-ui-xs"
	case n >= 4:
		return "text-ui-sm"
	default:
		return "text-ui"
	}
}

// StampNameFontPt 견적서 셀 글자 크기(pt).
func StampNameFontPt(name string) float64 {
	n := utf8.RuneCountInString(strings.TrimSpace(name))
	switch {
	case n >= 6:
		return 8
	case n >= 4:
		return 9
	default:
		return 11
	}
}
