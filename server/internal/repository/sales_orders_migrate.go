package repository

import (
	"database/sql"
	"log"
	"strings"
)

func applySalesOrders(db *sql.DB) {
	if db == nil {
		return
	}
	for _, q := range []string{
		`CREATE TABLE IF NOT EXISTS sales_orders (
			order_id         TEXT PRIMARY KEY,
			sales_id         TEXT NOT NULL DEFAULT '',
			quote_id         TEXT NOT NULL DEFAULT '',
			order_no         TEXT NOT NULL,
			order_date       TEXT NOT NULL,
			po_no            TEXT NOT NULL DEFAULT '',
			po_date          TEXT NOT NULL DEFAULT '',
			customer_id      TEXT NOT NULL DEFAULT '',
			recipient_name   TEXT NOT NULL DEFAULT '',
			title            TEXT NOT NULL DEFAULT '',
			due_date         TEXT NOT NULL DEFAULT '',
			vat_mode         TEXT NOT NULL DEFAULT 'excluded',
			amount           INTEGER NOT NULL DEFAULT 0,
			vat              INTEGER NOT NULL DEFAULT 0,
			total            INTEGER NOT NULL DEFAULT 0,
			status           TEXT NOT NULL DEFAULT 'open',
			billing_status   TEXT NOT NULL DEFAULT '',
			invoice_no       TEXT NOT NULL DEFAULT '',
			invoiced_at      TEXT NOT NULL DEFAULT '',
			paid_at          TEXT NOT NULL DEFAULT '',
			paid_amount      INTEGER NOT NULL DEFAULT 0,
			remarks          TEXT NOT NULL DEFAULT '',
			created_at       DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at       DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_sales_orders_no ON sales_orders(order_no)`,
		`CREATE INDEX IF NOT EXISTS idx_sales_orders_quote ON sales_orders(quote_id)`,
		`CREATE INDEX IF NOT EXISTS idx_sales_orders_sales ON sales_orders(sales_id)`,
		`CREATE INDEX IF NOT EXISTS idx_sales_orders_status ON sales_orders(status)`,
		`CREATE TABLE IF NOT EXISTS sales_order_lines (
			line_id           TEXT PRIMARY KEY,
			order_id          TEXT NOT NULL,
			quote_line_id     TEXT NOT NULL DEFAULT '',
			seq               INTEGER NOT NULL DEFAULT 1,
			item_id           TEXT NOT NULL DEFAULT '',
			group_label       TEXT NOT NULL DEFAULT '',
			name              TEXT NOT NULL,
			spec              TEXT NOT NULL DEFAULT '',
			qty               REAL NOT NULL DEFAULT 0,
			unit              TEXT NOT NULL DEFAULT 'EA',
			unit_price        INTEGER NOT NULL DEFAULT 0,
			amount            INTEGER NOT NULL DEFAULT 0,
			mm_rate           REAL NOT NULL DEFAULT 0,
			discount_rate     REAL NOT NULL DEFAULT 0,
			note              TEXT NOT NULL DEFAULT '',
			quote_qty         REAL NOT NULL DEFAULT 0,
			quote_unit_price  INTEGER NOT NULL DEFAULT 0,
			FOREIGN KEY (order_id) REFERENCES sales_orders(order_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_sales_order_lines_order ON sales_order_lines(order_id, seq)`,
		`CREATE TABLE IF NOT EXISTS sales_purchases (
			purchase_id    TEXT PRIMARY KEY,
			order_id       TEXT NOT NULL,
			line_id        TEXT NOT NULL DEFAULT '',
			supplier_name  TEXT NOT NULL DEFAULT '',
			supplier_id    TEXT NOT NULL DEFAULT '',
			purchase_date  TEXT NOT NULL DEFAULT '',
			qty            REAL NOT NULL DEFAULT 0,
			cost_price     INTEGER NOT NULL DEFAULT 0,
			eta_date       TEXT NOT NULL DEFAULT '',
			arrived_date   TEXT NOT NULL DEFAULT '',
			status         TEXT NOT NULL DEFAULT 'ordered',
			note           TEXT NOT NULL DEFAULT '',
			FOREIGN KEY (order_id) REFERENCES sales_orders(order_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_sales_purchases_order ON sales_purchases(order_id)`,
		`CREATE INDEX IF NOT EXISTS idx_sales_purchases_line ON sales_purchases(line_id)`,
		`INSERT OR IGNORE INTO codes (code_id, code_group, code_value, code_name, sort_order, is_active) VALUES
			('SOS01','sales_order_status','open','수주',1,1),
			('SOS02','sales_order_status','purchasing','발주',2,1),
			('SOS03','sales_order_status','ready','입고',3,1),
			('SOS04','sales_order_status','delivered','납품 완료',4,1),
			('SOS05','sales_order_status','closed','종료',5,1)`,
	} {
		if _, err := db.Exec(q); err != nil {
			if strings.Contains(err.Error(), "no such table") {
				return
			}
			log.Printf("038 sales_orders: %v", err)
		}
	}
	applySalesDeliveries(db)
}
