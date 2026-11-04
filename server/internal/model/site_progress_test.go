package model

import "testing"

func TestSiteCheckRateLabel(t *testing.T) {
	if SiteCheckRateLabel(0, 0) != "—" {
		t.Fatal("분모 0")
	}
	if SiteCheckRateLabel(1, 4) != "25%" {
		t.Fatal("1/4")
	}
}

func TestDatesInclusive(t *testing.T) {
	d := DatesInclusive("2026-11-01", "2026-11-03")
	if len(d) != 3 || d[2] != "2026-11-03" {
		t.Fatalf("%v", d)
	}
}

func TestTaskTracksSiteProgress(t *testing.T) {
	if TaskTracksSiteProgress(WBWorkAS, "") {
		t.Fatal("AS")
	}
	if TaskTracksSiteProgress(WBWorkAdmin, "maintenance") {
		t.Fatal("점검 원본")
	}
	if !TaskTracksSiteProgress(WBWorkProjectWork, "") {
		t.Fatal("작업")
	}
}

func TestValidProjectDutyPM(t *testing.T) {
	if !ValidProjectDuty(ProjectDutyPM) {
		t.Fatal("pm")
	}
	if ProjectDutyLabel(ProjectDutyPM) != "PM" {
		t.Fatal("label")
	}
}
