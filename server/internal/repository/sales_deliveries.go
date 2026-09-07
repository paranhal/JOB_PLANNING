package repository

import (
	"fmt"
	"strings"
	"time"

	"customer-support/internal/model"
)

func (r *OrderRepo) AddDelivery(d *model.SalesDelivery, serials []string, loc string) ([]model.Asset, error) {
	if d == nil || strings.TrimSpace(d.OrderID) == "" {
		return nil, fmt.Errorf("수주가 필요합니다")
	}
	o, err := r.Get(d.OrderID)
	if err != nil {
		return nil, err
	}
	var line model.SalesOrderLine
	found := false
	for _, ln := range o.Lines {
		if ln.LineID == strings.TrimSpace(d.LineID) {
			line = ln
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("수주 라인이 없습니다")
	}
	if d.Qty <= 0 {
		d.Qty = line.Qty
	}
	if strings.TrimSpace(d.DeliveryDate) == "" {
		d.DeliveryDate = time.Now().Format("2006-01-02")
	}
	var item *model.SalesItem
	if strings.TrimSpace(line.ItemID) != "" {
		item, _ = NewSalesItemRepo(r.db).Get(line.ItemID)
	}
	drafts, err := model.DeliveryAssetDrafts(o, line, item, *d, serials, loc)
	if err != nil {
		return nil, err
	}
	n, err := NextSeq(r.db, "sales_deliveries")
	if err != nil {
		return nil, err
	}
	d.DeliveryID = fmt.Sprintf("DL-%03d", n)
	d.LineID = line.LineID
	if _, err := r.db.Exec(`
		INSERT INTO sales_deliveries (
			delivery_id, order_id, line_id, delivery_date, qty, place, receiver_name, installed_by, note
		) VALUES (?,?,?,?,?,?,?,?,?)`,
		d.DeliveryID, d.OrderID, d.LineID, d.DeliveryDate, d.Qty, d.Place, d.ReceiverName, d.InstalledBy, d.Note); err != nil {
		return nil, err
	}
	assets := NewAssetRepo(r.db)
	var created []model.Asset
	for i := range drafts {
		a := drafts[i]
		if err := assets.Create(&a); err != nil {
			return created, err
		}
		created = append(created, a)
	}
	if o.Status != model.OrderStatusDelivered && o.Status != model.OrderStatusClosed {
		_ = r.SetStatus(o.OrderID, model.OrderStatusDelivered)
	}
	logCreate(r.db, "sales_deliveries", "delivery_id", d.DeliveryID, line.Name)
	return created, nil
}

func (r *OrderRepo) listDeliveries(orderID string) ([]model.SalesDelivery, error) {
	rows, err := r.db.Query(`
		SELECT d.delivery_id, d.order_id, COALESCE(d.line_id,''), d.delivery_date, COALESCE(d.qty,0),
			COALESCE(d.place,''), COALESCE(d.receiver_name,''), COALESCE(d.installed_by,''),
			COALESCE(d.note,''), COALESCE(d.created_at,''), COALESCE(l.name,'')
		FROM sales_deliveries d
		LEFT JOIN sales_order_lines l ON l.line_id=d.line_id
		WHERE d.order_id=? ORDER BY d.delivery_date, d.delivery_id`, orderID)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var items []model.SalesDelivery
	for rows.Next() {
		var d model.SalesDelivery
		if err := rows.Scan(&d.DeliveryID, &d.OrderID, &d.LineID, &d.DeliveryDate, &d.Qty,
			&d.Place, &d.ReceiverName, &d.InstalledBy, &d.Note, &d.CreatedAt, &d.LineName); err != nil {
			return nil, err
		}
		items = append(items, d)
	}
	return items, rows.Err()
}
