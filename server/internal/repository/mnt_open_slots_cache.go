package repository

import (
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	"customer-support/internal/model"
)

const openUnassignedSlotsTTL = 30 * time.Second

type openSlotsCacheEntry struct {
	at    time.Time
	slots []model.UnassignedMonthSlot
}

var (
	openSlotsCacheMu sync.Mutex
	openSlotsCache   = map[string]openSlotsCacheEntry{}
)

func openUnassignedSlotsCacheKey(db *sql.DB, planID string, year, month int) string {
	return fmt.Sprintf("%p|%s|%d|%d", db, planID, year, month)
}

func cloneUnassignedSlots(in []model.UnassignedMonthSlot) []model.UnassignedMonthSlot {
	if in == nil {
		return nil
	}
	out := make([]model.UnassignedMonthSlot, len(in))
	copy(out, in)
	return out
}

func getCachedOpenUnassignedSlots(db *sql.DB, planID string, year, month int) ([]model.UnassignedMonthSlot, bool) {
	key := openUnassignedSlotsCacheKey(db, planID, year, month)
	openSlotsCacheMu.Lock()
	defer openSlotsCacheMu.Unlock()
	ent, ok := openSlotsCache[key]
	if !ok {
		return nil, false
	}
	if time.Since(ent.at) >= openUnassignedSlotsTTL {
		delete(openSlotsCache, key)
		return nil, false
	}
	return cloneUnassignedSlots(ent.slots), true
}

func putCachedOpenUnassignedSlots(db *sql.DB, planID string, year, month int, slots []model.UnassignedMonthSlot) {
	key := openUnassignedSlotsCacheKey(db, planID, year, month)
	openSlotsCacheMu.Lock()
	defer openSlotsCacheMu.Unlock()
	openSlotsCache[key] = openSlotsCacheEntry{at: time.Now(), slots: cloneUnassignedSlots(slots)}
}

func invalidateOpenUnassignedSlotsCache() {
	openSlotsCacheMu.Lock()
	openSlotsCache = map[string]openSlotsCacheEntry{}
	openSlotsCacheMu.Unlock()
}

func (r *MaintenanceRepo) invalidateOpenUnassignedSlotsCache() {
	if r == nil || r.db == nil {
		invalidateOpenUnassignedSlotsCache()
		return
	}
	prefix := fmt.Sprintf("%p|", r.db)
	openSlotsCacheMu.Lock()
	defer openSlotsCacheMu.Unlock()
	for k := range openSlotsCache {
		if strings.HasPrefix(k, prefix) {
			delete(openSlotsCache, k)
		}
	}
}
