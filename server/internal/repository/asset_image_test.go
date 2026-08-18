package repository

import (
	"path/filepath"
	"testing"

	"customer-support/internal/model"
)

func TestAssetImageFileName(t *testing.T) {
	got := AssetImageFileName("AEZ-200E23-001", 2, "photo.PNG")
	want := "AEZ-200E23-001_img_2.png"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestAssetImageDir(t *testing.T) {
	got := AssetImageDir("data/uploads", "AEZ-200E23-001")
	want := filepath.Join("data/uploads", "assets", "AEZ-200E23-001")
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestReceiptPhotoDir(t *testing.T) {
	got := ReceiptPhotoDir("data/uploads", "AS-1")
	want := filepath.Join("data/uploads", "as", "AS-1", "receipt")
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestNextAssetImageSlot(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	repo := NewAttachmentRepo(db)
	assetID := "AEZ-TEST3-001"

	slot, err := repo.NextAssetImageSlot(assetID)
	if err != nil || slot != 1 {
		t.Fatalf("empty slot=%d err=%v", slot, err)
	}

	if err := repo.Create(mkAtt(assetID, 1)); err != nil {
		t.Fatal(err)
	}
	if err := repo.Create(mkAtt(assetID, 3)); err != nil {
		t.Fatal(err)
	}
	slot, err = repo.NextAssetImageSlot(assetID)
	if err != nil || slot != 2 {
		t.Fatalf("want 2 got %d err=%v", slot, err)
	}

	if err := repo.Create(mkAtt(assetID, 2)); err != nil {
		t.Fatal(err)
	}
	slot, err = repo.NextAssetImageSlot(assetID)
	if err != nil || slot != 0 {
		t.Fatalf("full want 0 got %d", slot)
	}
}

func TestUpdateKeywords(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := NewAttachmentRepo(db)
	a := mkAtt("AEZ-KW3-001", 1)
	a.Keywords = "초기"
	if err := repo.Create(a); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateKeywords(a.AttachmentID, "게이트, 정면"); err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetByID(a.AttachmentID)
	if err != nil || got == nil {
		t.Fatal(err)
	}
	if got.Keywords != "게이트, 정면" || got.SlotNo != 1 {
		t.Fatalf("%+v", got)
	}
}

func mkAtt(assetID string, slot int) *model.Attachment {
	return &model.Attachment{
		RefType:  "asset",
		RefID:    assetID,
		FileName: AssetImageFileName(assetID, slot, "x.jpg"),
		FilePath: filepath.Join("data/uploads/assets", assetID, AssetImageFileName(assetID, slot, "x.jpg")),
		SlotNo:   slot,
	}
}
