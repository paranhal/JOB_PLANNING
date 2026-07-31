package repository

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

var reUnsafePath = regexp.MustCompile(`[^\w.\-]+`)

// SanitizeAssetIDForPath 자산번호를 폴더/파일명에 안전한 형태로 정규화
func SanitizeAssetIDForPath(assetID string) string {
	s := strings.TrimSpace(assetID)
	s = strings.ReplaceAll(s, "/", "-")
	s = strings.ReplaceAll(s, "\\", "-")
	s = reUnsafePath.ReplaceAllString(s, "_")
	if s == "" {
		return "ASSET"
	}
	return s
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

// AssetImageRelPath uploads 아래 공개 URL용 상대 경로
func AssetImageRelPath(assetID string, slot int, origName string) string {
	safe := SanitizeAssetIDForPath(assetID)
	return filepath.ToSlash(filepath.Join("assets", safe, AssetImageFileName(assetID, slot, origName)))
}
