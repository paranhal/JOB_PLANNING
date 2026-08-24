package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"

	"github.com/joho/godotenv"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"customer-support/internal/audit"
	"customer-support/internal/auditlog"
	"customer-support/internal/backup"
	"customer-support/internal/config"
	"customer-support/internal/handler"
	"customer-support/internal/hwpx"
	"customer-support/internal/repository"
)

// 템플릿·정적 파일·SQLite 경로가 상대 경로이므로, 실행 위치와 무관하게 server 모듈 루트를 작업 디렉터리로 맞춘다.
func chdirToServerRoot() {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		log.Fatal("runtime.Caller 실패")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	if err := os.Chdir(root); err != nil {
		log.Fatalf("작업 디렉터리를 server 루트로 바꿀 수 없습니다 (%s): %v", root, err)
	}
}

func main() {
	chdirToServerRoot()
	if err := hwpx.WritePlaceholderTemplate(hwpx.DefaultTemplatePath()); err != nil {
		log.Printf("조치완료보고서 템플릿 준비 실패: %v", err)
	}
	_ = godotenv.Load()
	cfg := config.Load()

	db, err := repository.InitDB(cfg.DBPath)
	if err != nil {
		log.Fatalf("DB 초기화 실패: %v", err)
	}
	defer db.Close()

	audit.Init(db)

	accessPath := filepath.Join(filepath.Dir(cfg.DBPath), "audit", "access.db")
	if err := auditlog.Init(accessPath); err != nil {
		log.Printf("접속기록 DB 열기 실패: %v (업무는 계속됩니다)", err)
	} else {
		log.Printf("접속기록: %s (쓰기 전용)", accessPath)
	}
	defer auditlog.Close()

	// 휴무일 시드·캐시는 InitDB(008)와 handler.New 의 Calendar 가 담당한다. §23.13.2
	// 화면을 그릴 때 특일 API 를 부르지 않는다.

	// 기본 관리자 계정 확보
	userRepo := repository.NewUserRepo(db)
	userRepo.EnsureAdmin(handler.HashPassword("admin"))
	userRepo.EnsureObserver(handler.HashPassword("1234"))
	// 완료·종료 AS 수정 잠금 해제 비밀번호 시드 (기본: as-edit)
	_ = repository.NewSettingsRepo(db).EnsureASCompletedEditPassword(handler.HashPassword)
	if pw, err := repository.NewSettingsRepo(db).EnsureMaintenanceDeletePassword(handler.HashPassword); err != nil {
		log.Printf("정기점검 삭제 비밀번호 시드 실패: %v", err)
	} else if pw != "" {
		log.Printf("정기점검 계획 삭제 비밀번호 초기값: %s (관리 > 사용자에서 변경하세요)", pw)
	}

	backupCfg := backup.Config{
		DataDir: filepath.Dir(cfg.DBPath),
		DB:      db,
	}
	if n, err := backup.SyncIndex(backupCfg); err != nil {
		log.Printf("백업 목록 보정 실패: %v", err)
	} else if n > 0 {
		log.Printf("백업 목록 보정: %d건", n)
	}
	backup.Start(backupCfg)
	audit.StartArchiveScheduler(filepath.Dir(cfg.DBPath))

	e := echo.New()
	e.HideBanner = true

	e.HTTPErrorHandler = func(err error, c echo.Context) {
		log.Printf("오류 [%s %s]: %v", c.Request().Method, c.Request().URL.Path, err)
		e.DefaultHTTPErrorHandler(err, c)
	}

	e.Pre(handler.DecodePathMiddleware)

	e.Use(middleware.LoggerWithConfig(middleware.LoggerConfig{
		Format: "[${time_rfc3339}] ${method} ${uri} → ${status} (${latency_human})\n",
	}))
	e.Use(middleware.Recover())

	e.Renderer = handler.NewRenderer()
	e.Static("/static", "web/static")
	e.Static("/uploads", "data/uploads")

	h := handler.New(db)

	// 인증
	e.GET("/login", h.Auth.LoginPage)
	e.POST("/login", h.Auth.Login)
	e.GET("/logout", h.Auth.Logout)

	v1 := e.Group("/api/v1", h.Integration.RequireAPIKey)
	v1.GET("/customers", h.Integration.ListCustomers)
	v1.GET("/customers/:id/contacts", h.Integration.ListCustomerContacts)
	v1.GET("/customers/:id", h.Integration.GetCustomer)
	v1.GET("/contacts", h.Integration.ListContacts)
	v1.GET("/contacts/:id", h.Integration.GetContact)
	v1.GET("/codes", h.Integration.ListCodes)

	// 인증 미들웨어 적용 그룹
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.Use(h.Auth.RequireActiveRole)

	g.GET("/", h.Dashboard)
	g.GET("/work", h.Work.List)
	g.GET("/plan/unplanned", h.Work.UnplannedList)
	g.POST("/plan/unplanned/assign", h.Work.UnplannedAssign)
	g.POST("/plan/unplanned/no-date", h.Work.UnplannedNoDate)
	g.GET("/meeting", h.Meeting.Show)
	g.GET("/account", h.Auth.AccountPage)
	g.POST("/account/profile", h.Auth.AccountUpdateProfile)
	g.POST("/account/password", h.Auth.AccountChangePassword)

	adminOnly := h.Auth.RequireAdminMW
	masterWrite := h.Auth.RequireMasterWrite
	receiveAS := h.Auth.RequireReceiveAS
	processAS := h.Auth.RequireProcessAS

	// 고객 관리
	cust := g.Group("/customers")
	cust.GET("", h.Customer.List)
	cust.GET("/export.xlsx", h.Customer.ExportExcel)
	cust.GET("/new", h.Customer.New, masterWrite)
	cust.POST("", h.Customer.Create, masterWrite)
	cust.GET("/:id", h.Customer.Show)
	cust.GET("/:id/edit", h.Customer.Edit, masterWrite)
	cust.POST("/:id/update", h.Customer.Update, masterWrite)
	cust.POST("/:id/delete", h.Customer.Delete, masterWrite)
	cust.POST("/:id/review", h.Customer.ToggleReview, masterWrite)
	cust.GET("/:id/tab/assets", h.Customer.TabAssets)
	cust.GET("/:id/tab/contacts", h.Customer.TabContacts)
	cust.GET("/:id/tab/as", h.Customer.TabAS)

	// 공간 관리
	space := g.Group("/spaces", adminOnly)
	space.GET("", h.Space.List)
	space.POST("/buildings", h.Space.CreateBuilding)
	space.POST("/buildings/:id/update", h.Space.UpdateBuilding)
	space.POST("/buildings/:id/delete", h.Space.DeleteBuilding)
	space.POST("/floors", h.Space.CreateFloor)
	space.POST("/floors/:id/update", h.Space.UpdateFloor)
	space.POST("/floors/:id/delete", h.Space.DeleteFloor)
	space.POST("/rooms", h.Space.CreateRoom)
	space.POST("/rooms/batch", h.Space.BatchUpdateRooms)
	space.POST("/rooms/:id/update", h.Space.UpdateRoom)
	space.POST("/rooms/:id/delete", h.Space.DeleteRoom)

	api := g.Group("/api")
	api.GET("/buildings/:customer_id", h.Space.APIBuildings)
	api.GET("/floors/:building_id", h.Space.APIFloors)
	api.GET("/rooms/:floor_id", h.Space.APIRooms)
	api.GET("/assets/:customer_id", h.Asset.APIAssetsByCustomer)
	api.GET("/contacts/:customer_id", h.Contact.APIContactsByCustomer)
	api.GET("/as/history/:customer_id", h.AS.APIHistory)
	api.GET("/as/asset-history/:asset_id", h.AS.APIAssetHistory)

	contact := g.Group("/contacts")
	contact.GET("", h.Contact.List, adminOnly)
	contact.GET("/new", h.Contact.New, masterWrite)
	contact.POST("", h.Contact.Create, masterWrite)
	contact.GET("/:id", h.Contact.Show)
	contact.GET("/:id/edit", h.Contact.Edit, masterWrite)
	contact.POST("/:id/update", h.Contact.Update, masterWrite)

	g.GET("/contact-history", h.ContactHistory.List, adminOnly)
	g.POST("/contact-history", h.ContactHistory.Create, masterWrite)

	asset := g.Group("/assets")
	asset.GET("", h.Asset.List)
	asset.GET("/export.xlsx", h.Asset.ExportExcel)
	asset.GET("/new", h.Asset.New, masterWrite)
	asset.POST("", h.Asset.Create, masterWrite)
	asset.GET("/:id", h.Asset.Show)
	asset.GET("/:id/edit", h.Asset.Edit, masterWrite)
	asset.POST("/:id/update", h.Asset.Update, masterWrite)
	asset.POST("/:id/delete", h.Asset.Delete, masterWrite)
	asset.GET("/:asset_id/sw", h.SWDetail.ListByAsset)
	asset.POST("/:asset_id/sw", h.SWDetail.Create, masterWrite)
	g.GET("/sw/:id/edit", h.SWDetail.Edit, masterWrite)
	g.POST("/sw/:id/update", h.SWDetail.Update, masterWrite)
	g.POST("/sw/:id/delete", h.SWDetail.Delete, masterWrite)

	rel := g.Group("/relations", adminOnly)
	rel.GET("", h.Relation.List)
	rel.GET("/new", h.Relation.New)
	rel.POST("", h.Relation.Create)
	rel.GET("/:id/edit", h.Relation.Edit)
	rel.POST("/:id/update", h.Relation.Update)
	rel.POST("/:id/delete", h.Relation.Delete)

	as := g.Group("/as")
	as.GET("", h.AS.List)
	as.GET("/kanban", h.AS.Kanban)
	as.GET("/new", h.AS.New, receiveAS)
	as.POST("", h.AS.Create, receiveAS)
	as.GET("/stats", h.AS.StatsDashboard)
	// 하부업무 조치(원 접수와 독립) — /:id 보다 먼저 등록
	as.GET("/work/:work_id/action", h.AS.WorkAction)
	as.POST("/work/:work_id/action", h.AS.UpdateWorkAction)
	as.GET("/:id", h.AS.Show)
	as.GET("/:id/report", h.AS.ReportPreview)
	as.POST("/:id/report", h.AS.ReportIssue)
	as.GET("/:id/action", h.AS.Action) // 조회: 접수·업무 역할 / 수정은 핸들러·POST에서 제한
	as.POST("/:id/unlock-edit", h.AS.UnlockEdit, adminOnly)
	as.GET("/:id/edit", h.AS.Edit, receiveAS)
	as.POST("/:id/edit", h.AS.UpdateReceipt, receiveAS)
	as.POST("/:id/visit-date", h.AS.UpdateVisitDate)
	as.POST("/:id/update", h.AS.Update)
	as.POST("/:id/hold", h.AS.Hold, processAS)
	as.POST("/:id/hold-release", h.AS.ReleaseHold, processAS)
	as.POST("/:id/transfer", h.AS.Transfer, processAS)
	as.POST("/:id/transfer-complete", h.AS.CompleteTransfer, processAS)
	as.POST("/:id/cancel", h.AS.Cancel, processAS)
	as.POST("/:id/reopen", h.AS.Reopen)
	as.POST("/:id/process", h.AS.AddProcess, processAS)
	as.POST("/:id/process/:process_id/delete", h.AS.DeleteProcess, adminOnly)
	as.POST("/:id/delete", h.AS.Delete, adminOnly)

	ms := g.Group("/maintenance/sites", adminOnly)
	ms.GET("", h.Maintenance.ListSiteConfigs)
	ms.GET("/export.xlsx", h.Maintenance.ExportSiteConfigsExcel)
	ms.GET("/new", h.Maintenance.NewSiteConfigPage)
	ms.POST("/sync", h.Maintenance.SyncSiteConfigsFromAssets)
	ms.POST("", h.Maintenance.SaveSiteConfig)
	ms.GET("/:customer_id/edit", h.Maintenance.EditSiteConfigPage)
	ms.POST("/:customer_id/delete", h.Maintenance.DeleteSiteConfig)

	maint := g.Group("/maintenance")
	mntEdit := h.Auth.RequireMaintenanceEdit
	maint.GET("", h.Maintenance.ListPlans)
	maint.GET("/new", h.Maintenance.NewPlanPage, adminOnly)
	maint.POST("", h.Maintenance.CreatePlan, adminOnly)
	maint.GET("/visits/:visit_id/action", h.Maintenance.VisitAction)
	maint.POST("/visits/:visit_id/action", h.Maintenance.UpdateVisitAction, mntEdit)
	maint.POST("/visits/:visit_id/update", h.Maintenance.UpdateVisit, mntEdit)
	maint.POST("/visits/:visit_id/delete", h.Maintenance.DeleteVisit, mntEdit)
	maint.POST("/visits/:visit_id/complete", h.Maintenance.CompleteVisit, mntEdit)
	maint.POST("/:id/complete-until", h.Maintenance.CompleteVisitsUntil, mntEdit)
	maint.GET("/:id/export.xlsx", h.Maintenance.ExportExcel)
	maint.GET("/:id/generate", h.Maintenance.GenerateConfirm, adminOnly)
	maint.POST("/:id/generate", h.Maintenance.GenerateAuto, adminOnly)
	maint.POST("/:id/approve", h.Maintenance.ApprovePlan, adminOnly)
	maint.POST("/:id/unapprove", h.Maintenance.UnapprovePlan, adminOnly)
	maint.GET("/:id/delete", h.Maintenance.DeletePlanPage, adminOnly)
	maint.POST("/:id/delete", h.Maintenance.DeletePlan, adminOnly)
	maint.POST("/:id/archive", h.Maintenance.ArchivePlan, adminOnly)
	maint.GET("/:id/duplicates", h.Maintenance.DuplicatesPreview, adminOnly)
	maint.POST("/:id/duplicates", h.Maintenance.CollapseDuplicates, adminOnly)
	maint.GET("/:id/manage", h.Maintenance.ManagePlan)
	maint.POST("/:id/copy", h.Maintenance.CopyPlan, adminOnly)
	maint.POST("/:id/status", h.Maintenance.SetPlanStatus, adminOnly)
	maint.POST("/:id/bulk-assignee", h.Maintenance.BulkUpdateAssignees, adminOnly)
	maint.POST("/:id/bulk-delete", h.Maintenance.BulkDeleteVisits, adminOnly)
	maint.POST("/:id/assign-slots", h.Maintenance.AssignUnassignedSlots, adminOnly)
	maint.POST("/:id/visits", h.Maintenance.AddVisit, mntEdit)
	maint.POST("/:id/assign-slot", h.Maintenance.AssignUnassignedSlot, mntEdit)
	maint.GET("/:id", h.Maintenance.ShowPlan)

	g.GET("/analysis", h.Analysis.Dashboard, adminOnly)

	stats := g.Group("/stats")
	stats.GET("", h.Stats.Overview)
	stats.GET("/reports", h.Stats.Reports)
	stats.POST("/company-weekly", h.Stats.CreateCompanyWeekly)
	stats.GET("/company-weekly/download", h.Stats.DownloadCompanyWeekly)
	stats.GET("/detail", h.Stats.List)
	stats.GET("/export.xlsx", h.Stats.ExportExcel)
	stats.GET("/weekly-report.xlsx", h.Stats.ExportWeeklyReport)
	stats.GET("/daily-assignee.xlsx", h.Stats.ExportDailyAssigneeReport)

	g.GET("/work-status", h.WorkStatus.Calendar)

	proj := g.Group("/projects")
	proj.GET("", h.Project.List)
	proj.GET("/new", h.Project.New)
	proj.POST("", h.Project.Create)
	proj.GET("/:id", h.Project.Show)
	proj.GET("/:id/edit", h.Project.Edit)
	proj.POST("/:id", h.Project.Update)
	proj.POST("/:id/update", h.Project.Update)
	proj.POST("/:id/promote-customer", h.Project.PromoteCustomer)
	proj.POST("/:id/archive", h.Project.Archive)
	proj.POST("/:id/activate", h.Project.Activate)
	proj.POST("/:id/delete", h.Project.Delete)

	aw := g.Group("/admin-work")
	aw.GET("", h.AdminWork.List)
	aw.GET("/stats", h.AdminWork.Stats)
	aw.GET("/new", h.AdminWork.New)
	aw.POST("", h.AdminWork.Create)
	aw.POST("/inbox", h.AdminWork.CreateInbox)
	aw.POST("/:id/classify", h.AdminWork.Classify)
	aw.GET("/:id", h.AdminWork.Show)

	wb := g.Group("/workboard")
	wb.GET("", h.Workboard.Index)
	wb.GET("/kanban", h.Workboard.Kanban)
	wb.GET("/tasks", h.Workboard.List)
	wb.GET("/register", h.Workboard.Register)
	wb.POST("/register/unlock-past", h.Workboard.UnlockPastRegister, adminOnly)
	wb.POST("/projects", h.Workboard.CreateProject)
	wb.POST("/tasks", h.Workboard.CreateTask)
	wb.GET("/tasks/:id", h.Workboard.ShowTask)
	wb.POST("/tasks/:id/update", h.Workboard.UpdateTask)
	wb.POST("/tasks/:id/actions", h.Workboard.CreateAction)
	wb.POST("/tasks/:id/actions/:aid/update", h.Workboard.UpdateAction)
	wb.POST("/tasks/:id/activities", h.Workboard.CreateActivity)
	wb.POST("/tasks/:id/subtasks", h.Workboard.CreateSubtasks)
	wb.POST("/schedule", h.Workboard.Schedule)
	wb.POST("/unschedule", h.Workboard.Unschedule)

	g.GET("/codes", h.Code.List, adminOnly)
	g.POST("/codes", h.Code.Create, adminOnly)
	g.POST("/codes/:id/update", h.Code.Update, adminOnly)
	g.POST("/codes/:id/delete", h.Code.Delete, adminOnly)

	g.POST("/attachments", h.Attachment.Upload) // 권한은 Upload 내부에서 ref_type별 검사
	g.GET("/attachments/:id", h.Attachment.Download)
	g.POST("/attachments/:id/keywords", h.Attachment.UpdateKeywords)
	g.POST("/attachments/:id/promote-asset", h.Attachment.PromoteToAsset)
	g.POST("/attachments/:id/delete", h.Attachment.Delete)

	g.GET("/users", h.Auth.UserList, adminOnly)
	g.POST("/users/as-edit-password", h.AS.UpdateCompletedEditPassword, adminOnly)
	g.POST("/users/mnt-delete-password", h.Auth.UpdateMaintenanceDeletePassword, adminOnly)
	g.POST("/users", h.Auth.UserCreate, adminOnly)
	g.POST("/users/:id/update", h.Auth.UserUpdate, adminOnly)
	g.POST("/users/:id/password", h.Auth.UserChangePassword, adminOnly)

	g.GET("/admin/holidays", h.Holiday.List)
	g.POST("/admin/holidays", h.Holiday.Create)
	g.POST("/admin/holidays/update", h.Holiday.Update)
	g.POST("/admin/holidays/delete", h.Holiday.Delete)
	g.POST("/admin/holidays/sync", h.Holiday.SyncAPI)
	g.POST("/admin/holidays/leaves", h.Holiday.CreateLeave)
	g.POST("/admin/holidays/leaves/delete", h.Holiday.DeleteLeave)

	g.GET("/admin/data", h.Backup.Page, adminOnly)
	g.POST("/admin/data/save", h.Backup.Save, adminOnly)
	g.POST("/admin/data/rollback", h.Backup.Rollback, adminOnly)
	g.POST("/admin/data/archive", h.Backup.Archive, adminOnly)
	g.GET("/admin/backup", h.Backup.RedirectLegacy, adminOnly)
	g.POST("/admin/backup", h.Backup.Save, adminOnly)

	log.Printf("고객지원시스템 서버 시작: http://localhost:%s", cfg.Port)
	if err := e.Start(":" + cfg.Port); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
