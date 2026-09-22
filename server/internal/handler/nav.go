package handler

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
	NavSalesItems       = "sales_items"
	NavQuotes           = "quotes"
	NavOrders           = "orders"
	NavContracts        = "contracts"
	NavSalesLogs        = "sales_logs"
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
		NavSalesItems,
		NavQuotes,
		NavOrders,
		NavContracts,
		NavSalesLogs,
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
