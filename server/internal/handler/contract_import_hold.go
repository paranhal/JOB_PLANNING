package handler

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

const contractImportHoldTTL = 15 * time.Minute

type contractImportBlob struct {
	UserID  string
	OrgID   string
	Name    string
	Raw     []byte
	Applied bool
	Until   time.Time
}

type contractImportHold struct {
	mu sync.Mutex
	m  map[string]contractImportBlob
}

func newContractImportHold() *contractImportHold {
	return &contractImportHold{m: map[string]contractImportBlob{}}
}

func (h *contractImportHold) Put(userID, orgID, name string, raw []byte) string {
	if h == nil {
		return ""
	}
	tok := randomImportToken()
	h.mu.Lock()
	defer h.mu.Unlock()
	h.sweepLocked()
	h.m[tok] = contractImportBlob{
		UserID: userID, OrgID: orgID, Name: name, Raw: raw,
		Until: time.Now().Add(contractImportHoldTTL),
	}
	return tok
}

func (h *contractImportHold) Get(userID, orgID, tok string, consumeApply bool) ([]byte, string, error) {
	if h == nil || tok == "" {
		return nil, "", fmt.Errorf("미리보기가 없습니다")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.sweepLocked()
	b, ok := h.m[tok]
	if !ok {
		return nil, "", errImportHoldGone
	}
	if b.UserID != userID || b.OrgID != orgID {
		return nil, "", errImportHoldForbidden
	}
	if consumeApply {
		if b.Applied {
			return nil, "", errImportHoldUsed
		}
		b.Applied = true
		h.m[tok] = b
	}
	return b.Raw, b.Name, nil
}

var (
	errImportHoldGone      = fmt.Errorf("미리보기가 만료되었습니다")
	errImportHoldForbidden = fmt.Errorf("다른 사용자의 미리보기입니다")
	errImportHoldUsed      = fmt.Errorf("이미 적용한 미리보기입니다")
)

func (h *contractImportHold) sweepLocked() {
	now := time.Now()
	for k, v := range h.m {
		if now.After(v.Until) {
			delete(h.m, k)
		}
	}
}

func randomImportToken() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}
