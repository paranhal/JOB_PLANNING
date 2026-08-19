package handler

import (
	"customer-support/internal/model"
)

// PhotoGalleryVM 접수·조치 사진 갤러리. 템플릿 as_photo_gallery 가 쓴다. §12.9 · §12.9.8
type PhotoGalleryVM struct {
	AS                *model.ASReceipt
	Photos            []model.Attachment
	CanEditPhotos     bool
	CanPromote        bool
	AssetImageCount   int
	PhotoRedirect     string
	PhotoSectionTitle string
	PhotoPanelClass   string
	HideIfEmpty       bool
	PhotoMax          int
	PhotoRefType      string
	PhotoAccept       string
	PhotoHint         string
	PhotoEmpty        string
	PhotoAddLabel     string
	PhotoGalleryID    string
}

func (g PhotoGalleryVM) Max() int {
	if g.PhotoMax > 0 {
		return g.PhotoMax
	}
	return model.MaxReceiptPhotos
}

func (g PhotoGalleryVM) CanAdd() bool {
	return g.CanEditPhotos && len(g.Photos) < g.Max()
}

func (g PhotoGalleryVM) LightboxID() string {
	id := g.PhotoGalleryID
	if id == "" {
		id = "receipt"
	}
	return id + "-lightbox"
}

func receiptGalleryFromPage(as *model.ASReceipt, data map[string]interface{}) PhotoGalleryVM {
	photos, _ := data["ReceiptPhotos"].([]model.Attachment)
	canEdit, _ := data["CanEditPhotos"].(bool)
	canPromote, _ := data["CanPromote"].(bool)
	assetCount, _ := data["AssetImageCount"].(int)
	redirect, _ := data["PhotoRedirect"].(string)
	title, _ := data["PhotoSectionTitle"].(string)
	panel, _ := data["PhotoPanelClass"].(string)
	hide, _ := data["HideIfEmpty"].(bool)
	if title == "" {
		title = "증상 사진"
	}
	if redirect == "" && as != nil {
		redirect = "/as/" + as.ASID
	}
	return PhotoGalleryVM{
		AS:                as,
		Photos:            photos,
		CanEditPhotos:     canEdit,
		CanPromote:        canPromote,
		AssetImageCount:   assetCount,
		PhotoRedirect:     redirect,
		PhotoSectionTitle: title,
		PhotoPanelClass:   panel,
		HideIfEmpty:       hide,
		PhotoMax:          model.MaxReceiptPhotos,
		PhotoRefType:      model.RefTypeASReceipt,
		PhotoAccept:       "image/*,.heic,.heif,.pdf,.webp",
		PhotoHint:         "증상 사진 또는 자료 추가 (jpg · png · webp · heic · pdf)",
		PhotoEmpty:        "등록된 증상 사진이 없습니다.",
		PhotoAddLabel:     "사진 추가",
		PhotoGalleryID:    "receipt",
	}
}
