package model

import (
	"fmt"
	"strings"
)

type SalesDelivery struct {
	DeliveryID   string
	OrderID      string
	LineID       string
	DeliveryDate string
	Qty          float64
	Place        string
	ReceiverName string
	InstalledBy  string
	Note         string
	CreatedAt    string
	LineName     string
}

func SerialSlots(qty float64) int {
	if qty <= 0 {
		return 0
	}
	n := int(qty)
	if float64(n) < qty {
		n++
	}
	if n < 1 {
		return 1
	}
	return n
}

func PadSerials(serials []string, n int) []string {
	out := make([]string, n)
	for i := 0; i < n; i++ {
		if i < len(serials) {
			out[i] = strings.TrimSpace(serials[i])
		}
	}
	return out
}

func DeliveryAssetDrafts(o *SalesOrder, line SalesOrderLine, item *SalesItem, d SalesDelivery, serials []string, loc string) ([]Asset, error) {
	if o == nil {
		return nil, fmt.Errorf("수주가 필요합니다")
	}
	if item == nil {
		return nil, nil
	}
	kind, cat := item.ItemKind, item.Category
	modelName, mfr := line.Spec, item.Manufacturer
	if strings.TrimSpace(item.Model) != "" {
		modelName = item.Model
	}
	if !LineCreatesInstalledAsset(kind, cat) {
		return nil, nil
	}
	if strings.TrimSpace(o.CustomerID) == "" {
		return nil, fmt.Errorf("고객을 연결해야 납품 자산을 만들 수 있습니다")
	}
	n := SerialSlots(d.Qty)
	if n < 1 {
		n = 1
	}
	sns := PadSerials(serials, n)
	date := strings.TrimSpace(d.DeliveryDate)
	loc = strings.TrimSpace(loc)
	out := make([]Asset, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, Asset{
			CustomerID:        o.CustomerID,
			ProductName:       line.Name,
			ProductCategory:   cat,
			ModelName:         modelName,
			Manufacturer:      mfr,
			SerialNumber:      sns[i],
			InstallDate:       date,
			InstallerType:     InstallerTypeSelf,
			OriginalInstaller: strings.TrimSpace(d.InstalledBy),
			OperationStatus:   AssetOpOperating,
			InstallLocation:   loc,
			SalesOrderID:      o.OrderID,
		})
	}
	return out, nil
}
