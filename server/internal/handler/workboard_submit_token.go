package handler

import (
	"strings"
	"sync"
	"time"
)

type workSubmitEntry struct {
	taskID string
	at     time.Time
}

var workSubmitTokens = struct {
	sync.Mutex
	m map[string]workSubmitEntry
}{m: map[string]workSubmitEntry{}}

const workSubmitTokenTTL = 10 * time.Minute
const workSubmitRecentWindow = 5 * time.Second

func rememberWorkSubmitToken(token, taskID string) {
	token = strings.TrimSpace(token)
	taskID = strings.TrimSpace(taskID)
	if token == "" || taskID == "" {
		return
	}
	workSubmitTokens.Lock()
	defer workSubmitTokens.Unlock()
	now := time.Now()
	for k, v := range workSubmitTokens.m {
		if now.Sub(v.at) > workSubmitTokenTTL {
			delete(workSubmitTokens.m, k)
		}
	}
	workSubmitTokens.m[token] = workSubmitEntry{taskID: taskID, at: now}
}

func lookupWorkSubmitToken(token string) (string, bool) {
	token = strings.TrimSpace(token)
	if token == "" {
		return "", false
	}
	workSubmitTokens.Lock()
	defer workSubmitTokens.Unlock()
	ent, ok := workSubmitTokens.m[token]
	if !ok {
		return "", false
	}
	if time.Since(ent.at) > workSubmitTokenTTL {
		delete(workSubmitTokens.m, token)
		return "", false
	}
	return ent.taskID, true
}

func newWorkSubmitToken() string {
	return randomHexToken()
}

func attachWorkSubmitToken(data map[string]interface{}) {
	if data == nil {
		return
	}
	data["SubmitToken"] = newWorkSubmitToken()
}
