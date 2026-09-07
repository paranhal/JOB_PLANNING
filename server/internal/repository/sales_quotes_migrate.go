package repository

import (
	"database/sql"
	"log"
	"strings"
)

func applySalesQuotes(db *sql.DB) {
	if db == nil {
		return
	}
	for _, q := range []string{
		`CREATE TABLE IF NOT EXISTS sales_quotes (
			quote_id         TEXT PRIMARY KEY,
			sales_id         TEXT NOT NULL DEFAULT '',
			quote_no         TEXT NOT NULL,
			rev              INTEGER NOT NULL DEFAULT 0,
			form_type        TEXT NOT NULL DEFAULT 'A',
			recipient_kind   TEXT NOT NULL DEFAULT 'customer',
			customer_id      TEXT NOT NULL DEFAULT '',
			recipient_name   TEXT NOT NULL DEFAULT '',
			attn_name        TEXT NOT NULL DEFAULT '',
			attn_title       TEXT NOT NULL DEFAULT '',
			quote_date       TEXT NOT NULL,
			title            TEXT NOT NULL DEFAULT '',
			valid_until_text TEXT NOT NULL DEFAULT '',
			due_text         TEXT NOT NULL DEFAULT '',
			place_text       TEXT NOT NULL DEFAULT '',
			payment_text     TEXT NOT NULL DEFAULT '',
			vat_mode         TEXT NOT NULL DEFAULT 'excluded',
			round_rule       TEXT NOT NULL DEFAULT 'none',
			subtotal         INTEGER NOT NULL DEFAULT 0,
			vat              INTEGER NOT NULL DEFAULT 0,
			total            INTEGER NOT NULL DEFAULT 0,
			owner_user_id    TEXT NOT NULL DEFAULT '',
			owner_name       TEXT NOT NULL DEFAULT '',
			owner_phone      TEXT NOT NULL DEFAULT '',
			remarks          TEXT NOT NULL DEFAULT '',
			purpose          TEXT NOT NULL DEFAULT 'deal',
			budget_year      INTEGER NOT NULL DEFAULT 0,
			overhead_rate    REAL NOT NULL DEFAULT 0,
			tech_fee_rate    REAL NOT NULL DEFAULT 0,
			status           TEXT NOT NULL DEFAULT 'draft',
			is_legacy        INTEGER NOT NULL DEFAULT 0,
			rev_reason       TEXT NOT NULL DEFAULT '',
			is_reverse_calc  INTEGER NOT NULL DEFAULT 0,
			target_total     INTEGER NOT NULL DEFAULT 0,
			maint_block      INTEGER NOT NULL DEFAULT 0,
			created_at       DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at       DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_sales_quotes_no_rev ON sales_quotes(quote_no, rev)`,
		`CREATE INDEX IF NOT EXISTS idx_sales_quotes_sales ON sales_quotes(sales_id)`,
		`CREATE INDEX IF NOT EXISTS idx_sales_quotes_date ON sales_quotes(quote_date)`,
		`CREATE TABLE IF NOT EXISTS sales_quote_lines (
			line_id          TEXT PRIMARY KEY,
			quote_id         TEXT NOT NULL,
			seq              INTEGER NOT NULL DEFAULT 1,
			item_id          TEXT NOT NULL DEFAULT '',
			group_label      TEXT NOT NULL DEFAULT '',
			name             TEXT NOT NULL,
			spec             TEXT NOT NULL DEFAULT '',
			qty              REAL NOT NULL DEFAULT 0,
			unit             TEXT NOT NULL DEFAULT 'EA',
			unit_price       INTEGER NOT NULL DEFAULT 0,
			amount           INTEGER NOT NULL DEFAULT 0,
			gov_price        INTEGER NOT NULL DEFAULT 0,
			mm_rate          REAL NOT NULL DEFAULT 0,
			discount_rate    REAL NOT NULL DEFAULT 0,
			note             TEXT NOT NULL DEFAULT '',
			rate_id          TEXT NOT NULL DEFAULT '',
			labor_year       INTEGER NOT NULL DEFAULT 0,
			price_overridden INTEGER NOT NULL DEFAULT 0,
			FOREIGN KEY (quote_id) REFERENCES sales_quotes(quote_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_sales_quote_lines_quote ON sales_quote_lines(quote_id, seq)`,
	} {
		if _, err := db.Exec(q); err != nil {
			if strings.Contains(err.Error(), "no such table") {
				return
			}
			log.Printf("036 sales_quotes: %v", err)
		}
	}
	applyQuoteLabor(db)
	applySalesOrders(db)
}
