package repository

import (
	"database/sql"
	"log"
	"strings"
	"sync"

	"customer-support/internal/model"
)

var (
	lookupMu      sync.RWMutex
	customerNames = map[string]string{}
	userNames     = map[string]string{}
	codesCache    []model.Code
	lookupLoaded  bool
)

// LoadLookupCache 거의 안 바뀌는 표를 메모리에 올린다. 기동 시 1회. §44.7
func LoadLookupCache(db *sql.DB) {
	if db == nil {
		return
	}
	loadCustomerNames(db)
	loadUserNames(db)
	loadCodes(db)
	lookupMu.Lock()
	lookupLoaded = true
	lookupMu.Unlock()
}

func loadCustomerNames(db *sql.DB) {
	rows, err := db.Query(`SELECT customer_id, COALESCE(org_name,'') FROM customers`)
	if err != nil {
		log.Printf("lookup customers: %v", err)
		return
	}
	defer rows.Close()
	next := map[string]string{}
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			return
		}
		next[id] = name
	}
	lookupMu.Lock()
	customerNames = next
	lookupMu.Unlock()
}

func loadUserNames(db *sql.DB) {
	rows, err := db.Query(`SELECT user_id, COALESCE(full_name,'') FROM users`)
	if err != nil {
		log.Printf("lookup users: %v", err)
		return
	}
	defer rows.Close()
	next := map[string]string{}
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			return
		}
		next[id] = name
	}
	lookupMu.Lock()
	userNames = next
	lookupMu.Unlock()
}

func loadCodes(db *sql.DB) {
	items, err := scanCodesQuery(db, `SELECT code_id, code_group, code_value, code_name, sort_order, is_active
		 FROM codes ORDER BY code_group, sort_order`)
	if err != nil {
		log.Printf("lookup codes: %v", err)
		return
	}
	lookupMu.Lock()
	codesCache = items
	lookupMu.Unlock()
}

func scanCodesQuery(db *sql.DB, q string, args ...any) ([]model.Code, error) {
	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCodes(rows)
}

// CustomerName 고객 id → 기관명. 캐시 미적중 시 한 번만 DB 를 보고 채운다.
func CustomerName(db *sql.DB, id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}
	lookupMu.RLock()
	name, ok := customerNames[id]
	lookupMu.RUnlock()
	if ok {
		return name
	}
	if db == nil {
		return ""
	}
	_ = db.QueryRow(`SELECT COALESCE(org_name,'') FROM customers WHERE customer_id=?`, id).Scan(&name)
	if name != "" {
		rememberCustomerName(id, name)
	}
	return name
}

// UserName 사용자 id → 표시명.
func UserName(db *sql.DB, id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}
	lookupMu.RLock()
	name, ok := userNames[id]
	lookupMu.RUnlock()
	if ok {
		return name
	}
	if db == nil {
		return ""
	}
	_ = db.QueryRow(`SELECT COALESCE(full_name,'') FROM users WHERE user_id=?`, id).Scan(&name)
	if name != "" {
		rememberUserName(id, name)
	}
	return name
}

func rememberCustomerName(id, name string) {
	id = strings.TrimSpace(id)
	if id == "" {
		return
	}
	lookupMu.Lock()
	customerNames[id] = name
	lookupMu.Unlock()
}

func rememberUserName(id, name string) {
	id = strings.TrimSpace(id)
	if id == "" {
		return
	}
	lookupMu.Lock()
	userNames[id] = name
	lookupMu.Unlock()
}

func codesSnapshot() ([]model.Code, bool) {
	lookupMu.RLock()
	defer lookupMu.RUnlock()
	if !lookupLoaded && len(codesCache) == 0 {
		return nil, false
	}
	out := make([]model.Code, len(codesCache))
	copy(out, codesCache)
	return out, true
}

func reloadCodes(db *sql.DB) {
	if db == nil {
		return
	}
	loadCodes(db)
	lookupMu.Lock()
	lookupLoaded = true
	lookupMu.Unlock()
}
