package model

const (
	MilestoneDomainAS    = "as"
	MilestoneDomainMnt   = "mnt"
	MilestoneDomainAdmin = "admin"
	MilestoneDomainSales = "sales"

	MilestoneScopeOrg      = "org"
	MilestoneScopeAssignee = "assignee"
	MilestoneScopeDate     = "date"

	MilestoneDraft = "draft"
	MilestoneFixed = "fixed"
)

type MilestoneRow struct {
	WeekStart         string
	OrgID             string
	Domain            string
	Scope             string
	ScopeKey          string
	Received          int
	Processed         int
	Carried           int
	Minutes           int
	SalesNew          int
	SalesInfo         int
	SalesAct          int
	SalesQuote        int
	SalesOrder        int
	SalesContract     int
	SalesOpenDiscover int
	SalesOpenPropose  int
	SalesOpenBid      int
	WorkingDays       int
	State             string
	ComputedAt        string
	FixedAt           string
	FixedBy           string
}

type MilestoneEvidenceItem struct {
	ID     string
	Number string
	Title  string
	Date   string
	Href   string
}
