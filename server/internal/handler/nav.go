package handler

import (
	"strings"
)

// 사이드바 Active 키. base.html 의 $a 와 값이 같아야 한다. v2.0 §33.8
const (
	NavDashboard        = "dashboard"
	NavWorkRegister     = "work_register"
	NavPlanUnplanned    = "plan_unplanned"
	NavMaintenance      = "maintenance"
	NavMeeting          = "meeting"
	NavWork             = "work"
	NavWorkAll          = "work_all"
	NavWorkPlan         = "work_plan"
	NavAS               = "as"
	NavASStats          = "as_stats"
	NavASKnowledge      = "as_knowledge"
	NavAdminWork        = "admin_work"
	NavAdminWorkStats   = "admin_work_stats"
	NavSales            = "sales"
	NavSalesDashboard   = "sales_dashboard"
	NavSalesActivities  = "sales_activities"
	NavSalesPipeline    = "sales_pipeline"
	NavSalesItems       = "sales_items"
	NavQuotes           = "quotes"
	NavOrders           = "orders"
	NavContracts        = "contracts"
	NavWorkStatus       = "work_status"
	NavStats            = "stats"
	NavStatsReports     = "stats_reports"
	NavStatsDetail      = "stats_detail"
	NavAnalysis         = "analysis"
	NavCustomers        = "customers"
	NavSpaces           = "spaces"
	NavContacts         = "contacts"
	NavContactHistory   = "contact_history"
	NavAssets           = "assets"
	NavRelations        = "relations"
	NavProjects         = "projects"
	NavMaintenanceSites = "maintenance_sites"
	NavRegionOrder      = "region_order"
	NavCodes            = "codes"
	NavUsers            = "users"
	NavHolidays         = "holidays"
	NavData             = "data"
	NavBackup           = "backup"
	NavSystem           = "system"
	NavAccount          = "account"
	NavLogin            = "login"
	NavNone             = ""
)

func navSidebarKeys() []string {
	return []string{
		NavDashboard,
		NavWorkRegister,
		NavPlanUnplanned,
		NavMaintenance,
		NavMeeting,
		NavWork,
		NavWorkAll,
		NavWorkPlan,
		NavAS,
		NavASStats,
		NavASKnowledge,
		NavAdminWork,
		NavAdminWorkStats,
		NavSales,
		NavSalesDashboard,
		NavSalesActivities,
		NavSalesPipeline,
		NavSalesItems,
		NavQuotes,
		NavOrders,
		NavContracts,
		NavWorkStatus,
		NavStats,
		NavStatsReports,
		NavStatsDetail,
		NavAnalysis,
		NavCustomers,
		NavSpaces,
		NavContacts,
		NavContactHistory,
		NavAssets,
		NavRelations,
		NavProjects,
		NavMaintenanceSites,
		NavRegionOrder,
		NavCodes,
		NavUsers,
		NavHolidays,
		NavData,
		NavBackup,
		NavSystem,
		NavAccount,
	}
}

// IsSalesNav 영업 전용 사이드바. /sales* · /quotes* · /orders* · /contracts* · 고객 영업 보기. §46.3
func IsSalesNav(path, rawQuery string) bool {
	path = strings.TrimSpace(path)
	if path == "" {
		path = "/"
	}
	if path == "/sales" || strings.HasPrefix(path, "/sales/") {
		return true
	}
	if path == "/quotes" || strings.HasPrefix(path, "/quotes/") {
		return true
	}
	if path == "/orders" || strings.HasPrefix(path, "/orders/") {
		return true
	}
	if path == "/contracts" || strings.HasPrefix(path, "/contracts/") {
		return true
	}
	if path == "/customers" && strings.Contains(rawQuery, "view=sales") {
		return true
	}
	return false
}
