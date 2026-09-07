package repository

import (
	"database/sql"
	"log"
	"strings"
)

func applySalesDeliveries(db *sql.DB) {
	if db == nil {
		return
	}
	for _, q := range []string{
		`CREATE TABLE IF NOT EXISTS sales_deliveries (
			delivery_id    TEXT PRIMARY KEY,
			order_id       TEXT NOT NULL,
			line_id        TEXT NOT NULL DEFAULT '',
			delivery_date  TEXT NOT NULL,
			qty            REAL NOT NULL DEFAULT 0,
			place          TEXT NOT NULL DEFAULT '',
			receiver_name  TEXT NOT NULL DEFAULT '',
			installed_by   TEXT NOT NULL DEFAULT '',
			note           TEXT NOT NULL DEFAULT '',
			created_at     DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (order_id) REFERENCES sales_orders(order_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_sales_deliveries_order ON sales_deliveries(order_id)`,
		`CREATE INDEX IF NOT EXISTS idx_sales_deliveries_line ON sales_deliveries(line_id)`,
		`ALTER TABLE assets ADD COLUMN sales_order_id TEXT NOT NULL DEFAULT ''`,
		`CREATE INDEX IF NOT EXISTS idx_assets_sales_order ON assets(sales_order_id)`,
	} {
		if _, err := db.Exec(q); err != nil {
			msg := err.Error()
			if strings.Contains(msg, "duplicate column name") || strings.Contains(msg, "already exists") {
				continue
			}
			if strings.Contains(msg, "no such table") {
				return
			}
			log.Printf("039 sales_deliveries: %v", err)
		}
	}
}
