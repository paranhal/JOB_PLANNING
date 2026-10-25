package handler

import (
	"strings"
	"sync"
	"time"
)

type asSubmitEntry struct {
	ids []string
	at  time.Time
}

var asSubmitTokens = struct {
	sync.Mutex
	m map[string]asSubmitEntry
}{m: map[string]asSubmitEntry{}}

const asSubmitTokenTTL = 10 * time.Minute

func rememberASSubmitToken(token string, ids []string) {
	token = strings.TrimSpace(token)
	if token == "" || len(ids) == 0 {
		return
	}
	asSubmitTokens.Lock()
	defer asSubmitTokens.Unlock()
	now := time.Now()
	for k, v := range asSubmitTokens.m {
		if now.Sub(v.at) > asSubmitTokenTTL {
			delete(asSubmitTokens.m, k)
		}
	}
	cp := append([]string(nil), ids...)
	asSubmitTokens.m[token] = asSubmitEntry{ids: cp, at: now}
}

func lookupASSubmitToken(token string) ([]string, bool) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, false
	}
	asSubmitTokens.Lock()
	defer asSubmitTokens.Unlock()
	ent, ok := asSubmitTokens.m[token]
	if !ok {
		return nil, false
	}
	if time.Since(ent.at) > asSubmitTokenTTL {
		delete(asSubmitTokens.m, token)
		return nil, false
	}
	return append([]string(nil), ent.ids...), true
}

func newASSubmitToken() string {
	return randomHexToken()
}
