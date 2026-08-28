package handler

// 사이드바 Active 키. base.html 의 $a 와 값이 같아야 한다. v2.0 §33.8
const (
	NavDashboard        = "dashboard"
	NavWorkRegister     = "work_register"
	NavPlanUnplanned    = "plan_unplanned"
	NavMaintenance      = "maintenance"
	NavMeeting          = "meeting"
	NavWork             = "work"
	NavWorkPlan         = "work_plan"
	NavAS               = "as"
	NavASStats          = "as_stats"
	NavAdminWork        = "admin_work"
	NavAdminWorkStats   = "admin_work_stats"
	NavSales            = "sales"
	NavSalesActivities  = "sales_activities"
	NavSalesPipeline    = "sales_pipeline"
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
	NavCodes            = "codes"
	NavUsers            = "users"
	NavHolidays         = "holidays"
	NavData             = "data"
	NavBackup           = "backup"
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
		NavWorkPlan,
		NavAS,
		NavASStats,
		NavAdminWork,
		NavAdminWorkStats,
		NavSales,
		NavSalesActivities,
		NavSalesPipeline,
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
		NavCodes,
		NavUsers,
		NavHolidays,
		NavData,
		NavBackup,
		NavAccount,
	}
}
