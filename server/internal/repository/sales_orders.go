package repository

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"customer-support/internal/model"
)

type OrderRepo struct {
	db *sql.DB
}

func NewOrderRepo(db *sql.DB) *OrderRepo {
	return &OrderRepo{db: db}
}

const salesOrderSelect = `
	SELECT order_id, COALESCE(sales_id,''), COALESCE(quote_id,''), order_no, order_date,
		COALESCE(po_no,''), COALESCE(po_date,''), COALESCE(customer_id,''),
		COALESCE(recipient_name,''), COALESCE(title,''), COALESCE(due_date,''),
		COALESCE(vat_mode,'excluded'), COALESCE(amount,0), COALESCE(vat,0), COALESCE(total,0),
		COALESCE(status,'open'), COALESCE(billing_status,''), COALESCE(invoice_no,''),
		COALESCE(invoiced_at,''), COALESCE(paid_at,''), COALESCE(paid_amount,0),
		COALESCE(remarks,''), COALESCE(created_at,''), COALESCE(updated_at,'')
	FROM sales_orders`

type OrderFilter struct {
	Search string
	Status string
}

func (r *OrderRepo) List(f OrderFilter) ([]model.SalesOrder, error) {
	q := salesOrderSelect + ` WHERE 1=1`
	var args []interface{}
	if s := strings.TrimSpace(f.Search); s != "" {
		like := "%" + s + "%"
		q += ` AND (order_no LIKE ? OR COALESCE(title,'') LIKE ? OR COALESCE(recipient_name,'') LIKE ? OR COALESCE(po_no,'') LIKE ?)`
		args = append(args, like, like, like, like)
	}
	if s := strings.TrimSpace(f.Status); s != "" {
		q += ` AND status=?`
		args = append(args, model.NormalizeOrderStatus(s))
	}
	q += ` ORDER BY order_date DESC, order_no DESC`
	rows, err := r.db.Query(q, args...)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	items, err := scanOrders(rows)
	if err != nil {
		return nil, err
	}
	for i := range items {
		items[i].Lines, _ = r.listLines(items[i].OrderID)
		items[i].Purchases, _ = r.listPurchases(items[i].OrderID)
		items[i].ApplyCosts()
	}
	return items, nil
}

func (r *OrderRepo) Get(id string) (*model.SalesOrder, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, sql.ErrNoRows
	}
	row := r.db.QueryRow(salesOrderSelect+` WHERE order_id=?`, id)
	o, err := scanOrder(row)
	if err != nil {
		return nil, err
	}
	o.Lines, err = r.listLines(o.OrderID)
	if err != nil {
		return nil, err
	}
	o.Purchases, err = r.listPurchases(o.OrderID)
	if err != nil {
		return nil, err
	}
	o.Deliveries, _ = r.listDeliveries(o.OrderID)
	o.Assets, _ = NewAssetRepo(r.db).ListBySalesOrder(o.OrderID)
	o.ApplyCosts()
	return o, nil
}

func (r *OrderRepo) ListByQuote(quoteID string) ([]model.SalesOrder, error) {
	quoteID = strings.TrimSpace(quoteID)
	if quoteID == "" {
		return nil, nil
	}
	rows, err := r.db.Query(salesOrderSelect+` WHERE quote_id=? ORDER BY order_date DESC, order_no DESC`, quoteID)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	return scanOrders(rows)
}

func (r *OrderRepo) CreateFromQuote(q *model.SalesQuote) (*model.SalesOrder, error) {
	if q == nil || strings.TrimSpace(q.QuoteID) == "" {
		return nil, model.ErrOrderQuoteRequired
	}
	lines := model.OrderLinesFromQuote(q)
	if len(lines) == 0 {
		return nil, model.ErrOrderNoLines
	}
	today := time.Now().Format("2006-01-02")
	o := &model.SalesOrder{
		SalesID:       q.SalesID,
		QuoteID:       q.QuoteID,
		OrderDate:     today,
		CustomerID:    q.CustomerID,
		RecipientName: q.RecipientName,
		Title:         q.Title,
		DueDate:       q.DueText,
		VATMode:       q.VATMode,
		Status:        model.OrderStatusOpen,
		Lines:         lines,
	}
	model.ApplyOrderTotals(o)
	if err := r.insertOrder(o); err != nil {
		return nil, err
	}
	return o, nil
}

func (r *OrderRepo) insertOrder(o *model.SalesOrder) error {
	n, err := NextSeq(r.db, "sales_orders")
	if err != nil {
		return err
	}
	o.OrderID = fmt.Sprintf("SO-%03d", n)
	if strings.TrimSpace(o.OrderDate) == "" {
		o.OrderDate = time.Now().Format("2006-01-02")
	}
	seq, err := r.nextSeqForDay(o.OrderDate)
	if err != nil {
		return err
	}
	o.OrderNo = model.FormatOrderNo(o.OrderDate, seq)
	o.Status = model.NormalizeOrderStatus(o.Status)
	model.ApplyOrderTotals(o)
	_, err = r.db.Exec(`
		INSERT INTO sales_orders (
			order_id, sales_id, quote_id, order_no, order_date, po_no, po_date,
			customer_id, recipient_name, title, due_date, vat_mode, amount, vat, total,
			status, remarks
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		o.OrderID, o.SalesID, o.QuoteID, o.OrderNo, o.OrderDate, o.PONo, o.PODate,
		o.CustomerID, o.RecipientName, o.Title, o.DueDate, o.VATMode, o.Amount, o.VAT, o.Total,
		o.Status, o.Remarks)
	if err != nil {
		if isUniqueErr(err) {
			return fmt.Errorf("같은 수주번호가 이미 있습니다")
		}
		return err
	}
	if err := r.replaceLines(o); err != nil {
		return err
	}
	logCreate(r.db, "sales_orders", "order_id", o.OrderID, o.OrderNo)
	return nil
}

func (r *OrderRepo) Update(o *model.SalesOrder) error {
	if o == nil || strings.TrimSpace(o.OrderID) == "" {
		return fmt.Errorf("order_id 필요")
	}
	cur, err := r.Get(o.OrderID)
	if err != nil {
		return err
	}
	o.OrderNo = cur.OrderNo
	o.OrderDate = cur.OrderDate
	o.QuoteID = cur.QuoteID
	o.Status = model.NormalizeOrderStatus(o.Status)
	if o.Status == "" {
		o.Status = cur.Status
	}
	model.ApplyOrderTotals(o)
	return touchUpdate(r.db, "sales_orders", "order_id", o.OrderID, o.OrderNo, func() error {
		_, err := r.db.Exec(`
			UPDATE sales_orders SET
				sales_id=?, po_no=?, po_date=?, customer_id=?, recipient_name=?, title=?,
				due_date=?, vat_mode=?, amount=?, vat=?, total=?, status=?, remarks=?,
				updated_at=CURRENT_TIMESTAMP
			WHERE order_id=?`,
			o.SalesID, o.PONo, o.PODate, o.CustomerID, o.RecipientName, o.Title,
			o.DueDate, o.VATMode, o.Amount, o.VAT, o.Total, o.Status, o.Remarks, o.OrderID)
		if err != nil {
			return err
		}
		return r.updateLinePrices(o)
	})
}

func (r *OrderRepo) SetStatus(id, status string) error {
	o, err := r.Get(id)
	if err != nil {
		return err
	}
	o.Status = model.NormalizeOrderStatus(status)
	_, err = r.db.Exec(`UPDATE sales_orders SET status=?, updated_at=CURRENT_TIMESTAMP WHERE order_id=?`, o.Status, o.OrderID)
	return err
}

func (r *OrderRepo) AddPurchase(p *model.SalesPurchase) error {
	if p == nil || strings.TrimSpace(p.OrderID) == "" {
		return fmt.Errorf("수주가 필요합니다")
	}
	n, err := NextSeq(r.db, "sales_purchases")
	if err != nil {
		return err
	}
	p.PurchaseID = fmt.Sprintf("PU-%03d", n)
	p.Status = model.NormalizePurchaseStatus(p.Status)
	if strings.TrimSpace(p.ArrivedDate) != "" {
		p.Status = model.PurchaseStatusArrived
	}
	if strings.TrimSpace(p.PurchaseDate) == "" {
		p.PurchaseDate = time.Now().Format("2006-01-02")
	}
	_, err = r.db.Exec(`
		INSERT INTO sales_purchases (
			purchase_id, order_id, line_id, supplier_name, supplier_id, purchase_date,
			qty, cost_price, eta_date, arrived_date, status, note
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		p.PurchaseID, p.OrderID, p.LineID, p.SupplierName, p.SupplierID, p.PurchaseDate,
		p.Qty, p.CostPrice, p.EtaDate, p.ArrivedDate, p.Status, p.Note)
	if err != nil {
		return err
	}
	o, err := r.Get(p.OrderID)
	if err == nil && o != nil && o.Status == model.OrderStatusOpen {
		_ = r.SetStatus(p.OrderID, model.OrderStatusPurchasing)
	}
	logCreate(r.db, "sales_purchases", "purchase_id", p.PurchaseID, p.SupplierName)
	return nil
}

func (r *OrderRepo) nextSeqForDay(iso string) (int, error) {
	prefix := model.FormatOrderNo(iso, 1)
	if prefix == "" {
		return 0, fmt.Errorf("수주일자가 올바르지 않습니다")
	}
	dayPart := strings.TrimSuffix(prefix, "-001")
	like := dayPart + "-%"
	rows, err := r.db.Query(`SELECT order_no FROM sales_orders WHERE order_no LIKE ?`, like)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	max := 0
	for rows.Next() {
		var no string
		if err := rows.Scan(&no); err != nil {
			return 0, err
		}
		i := strings.LastIndex(no, "-")
		if i < 0 {
			continue
		}
		var seq int
		fmt.Sscanf(no[i+1:], "%d", &seq)
		if seq > max {
			max = seq
		}
	}
	return max + 1, rows.Err()
}

func (r *OrderRepo) replaceLines(o *model.SalesOrder) error {
	if _, err := r.db.Exec(`DELETE FROM sales_order_lines WHERE order_id=?`, o.OrderID); err != nil {
		return err
	}
	for i := range o.Lines {
		ln := &o.Lines[i]
		n, err := NextSeq(r.db, "sales_order_lines")
		if err != nil {
			return err
		}
		ln.LineID = fmt.Sprintf("OL-%03d", n)
		ln.OrderID = o.OrderID
		ln.Seq = i + 1
		ln.Amount = ln.CalcAmount()
		if _, err := r.db.Exec(`
			INSERT INTO sales_order_lines (
				line_id, order_id, quote_line_id, seq, item_id, group_label, name, spec,
				qty, unit, unit_price, amount, mm_rate, discount_rate, note,
				quote_qty, quote_unit_price
			) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			ln.LineID, ln.OrderID, ln.QuoteLineID, ln.Seq, ln.ItemID, ln.GroupLabel, ln.Name, ln.Spec,
			ln.Qty, ln.Unit, ln.UnitPrice, ln.Amount, ln.MMRate, ln.DiscountRate, ln.Note,
			ln.QuoteQty, ln.QuoteUnitPrice); err != nil {
			return err
		}
	}
	return nil
}

func (r *OrderRepo) updateLinePrices(o *model.SalesOrder) error {
	for i := range o.Lines {
		ln := &o.Lines[i]
		if strings.TrimSpace(ln.LineID) == "" {
			continue
		}
		ln.Amount = ln.CalcAmount()
		ln.Seq = i + 1
		if _, err := r.db.Exec(`
			UPDATE sales_order_lines SET
				qty=?, unit=?, unit_price=?, amount=?, mm_rate=?, discount_rate=?, note=?, seq=?
			WHERE line_id=? AND order_id=?`,
			ln.Qty, ln.Unit, ln.UnitPrice, ln.Amount, ln.MMRate, ln.DiscountRate, ln.Note, ln.Seq,
			ln.LineID, o.OrderID); err != nil {
			return err
		}
	}
	return nil
}

func (r *OrderRepo) listLines(orderID string) ([]model.SalesOrderLine, error) {
	rows, err := r.db.Query(`
		SELECT line_id, order_id, COALESCE(quote_line_id,''), seq, COALESCE(item_id,''),
			COALESCE(group_label,''), name, COALESCE(spec,''), COALESCE(qty,0), COALESCE(unit,'EA'),
			COALESCE(unit_price,0), COALESCE(amount,0), COALESCE(mm_rate,0), COALESCE(discount_rate,0),
			COALESCE(note,''), COALESCE(quote_qty,0), COALESCE(quote_unit_price,0)
		FROM sales_order_lines WHERE order_id=? ORDER BY seq, line_id`, orderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var lines []model.SalesOrderLine
	for rows.Next() {
		var ln model.SalesOrderLine
		if err := rows.Scan(&ln.LineID, &ln.OrderID, &ln.QuoteLineID, &ln.Seq, &ln.ItemID,
			&ln.GroupLabel, &ln.Name, &ln.Spec, &ln.Qty, &ln.Unit, &ln.UnitPrice, &ln.Amount,
			&ln.MMRate, &ln.DiscountRate, &ln.Note, &ln.QuoteQty, &ln.QuoteUnitPrice); err != nil {
			return nil, err
		}
		lines = append(lines, ln)
	}
	return lines, rows.Err()
}

func (r *OrderRepo) listPurchases(orderID string) ([]model.SalesPurchase, error) {
	rows, err := r.db.Query(`
		SELECT purchase_id, order_id, COALESCE(line_id,''), COALESCE(supplier_name,''),
			COALESCE(supplier_id,''), COALESCE(purchase_date,''), COALESCE(qty,0), COALESCE(cost_price,0),
			COALESCE(eta_date,''), COALESCE(arrived_date,''), COALESCE(status,'ordered'), COALESCE(note,'')
		FROM sales_purchases WHERE order_id=? ORDER BY purchase_date, purchase_id`, orderID)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var items []model.SalesPurchase
	for rows.Next() {
		var p model.SalesPurchase
		if err := rows.Scan(&p.PurchaseID, &p.OrderID, &p.LineID, &p.SupplierName, &p.SupplierID,
			&p.PurchaseDate, &p.Qty, &p.CostPrice, &p.EtaDate, &p.ArrivedDate, &p.Status, &p.Note); err != nil {
			return nil, err
		}
		items = append(items, p)
	}
	return items, rows.Err()
}

type orderScanner interface {
	Scan(dest ...interface{}) error
}

func scanOrder(row orderScanner) (*model.SalesOrder, error) {
	var o model.SalesOrder
	err := row.Scan(
		&o.OrderID, &o.SalesID, &o.QuoteID, &o.OrderNo, &o.OrderDate,
		&o.PONo, &o.PODate, &o.CustomerID, &o.RecipientName, &o.Title, &o.DueDate,
		&o.VATMode, &o.Amount, &o.VAT, &o.Total, &o.Status,
		&o.BillingStatus, &o.InvoiceNo, &o.InvoicedAt, &o.PaidAt, &o.PaidAmount,
		&o.Remarks, &o.CreatedAt, &o.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	o.Status = model.NormalizeOrderStatus(o.Status)
	return &o, nil
}

func scanOrders(rows *sql.Rows) ([]model.SalesOrder, error) {
	var items []model.SalesOrder
	for rows.Next() {
		o, err := scanOrder(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *o)
	}
	return items, rows.Err()
}
