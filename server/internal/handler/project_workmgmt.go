package handler

type workPick struct {
	ID   string
	Name string
	Kind string
}

type workStage struct {
	ID        string
	Kind      string
	Stage     string
	PlanStart string
	PlanEnd   string
	Actual    string
	Owner     string
	Status    string
}

type workDoc struct {
	Title  string
	Date   string
	Status string
	Action string
}

type workDocGroup struct {
	Kind  string
	Label string
	Count string
	Docs  []workDoc
}

type workMgmtSeed struct {
	DateLabel string
	Picks     []workPick
	Stages    []workStage
	DocGroups []workDocGroup
}
