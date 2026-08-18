package repository

import (
	"fmt"
	"hash/fnv"
	"path/filepath"
	"regexp"
	"strings"
)

var reUnsafePath = regexp.MustCompile(`[^\w.\-]+`)

// SanitizeAssetIDForPath 자산번호를 폴더/파일명에 안전한 형태로 정규화한다.
//
// Go 정규식의 \w는 ASCII만 포함해서 한글이 든 자산번호는 통째로 _ 로 바뀐다.
// 그러면 "A자료검색대3-001"과 "A로봇3-001"이 같은 폴더명이 되어 서로의 사진을
// 덮어쓰므로, 치환이 일어난 경우에만 원본 해시를 덧붙여 구분한다.
// ASCII 자산번호는 치환이 없어 기존 경로가 그대로 유지된다.
func SanitizeAssetIDForPath(assetID string) string {
	s := strings.TrimSpace(assetID)
	if s == "" {
		return "ASSET"
	}
	safe := strings.ReplaceAll(s, "/", "-")
	safe = strings.ReplaceAll(safe, "\\", "-")
	safe = reUnsafePath.ReplaceAllString(safe, "_")
	if safe == s {
		return safe
	}
	h := fnv.New32a()
	h.Write([]byte(s))
	return fmt.Sprintf("%s-%08x", strings.Trim(safe, "_"), h.Sum32())
}

// AssetImageDir 자산 이미지 저장 폴더 (uploadRoot 기준 상대/절대 조합용 상대 경로)
func AssetImageDir(uploadRoot, assetID string) string {
	return filepath.Join(uploadRoot, "assets", SanitizeAssetIDForPath(assetID))
}

// AssetImageFileName 파일명: {자산번호}_img_{N}{ext}
func AssetImageFileName(assetID string, slot int, origName string) string {
	ext := strings.ToLower(filepath.Ext(origName))
	if ext == "" {
		ext = ".jpg"
	}
	return fmt.Sprintf("%s_img_%d%s", SanitizeAssetIDForPath(assetID), slot, ext)
}

// ReceiptPhotoDir 접수 사진 저장 폴더. data/uploads/as/{접수ID}/receipt/
func ReceiptPhotoDir(uploadRoot, asID string) string {
	return filepath.Join(uploadRoot, "as", asID, "receipt")
}

// AssetImageRelPath uploads 아래 공개 URL용 상대 경로
func AssetImageRelPath(assetID string, slot int, origName string) string {
	safe := SanitizeAssetIDForPath(assetID)
	return filepath.ToSlash(filepath.Join("assets", safe, AssetImageFileName(assetID, slot, origName)))
}
