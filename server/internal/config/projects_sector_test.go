package config

import "testing"

func TestShowProjectsSectorFollowsAppEnv(t *testing.T) {
	t.Setenv("SHOW_PROJECTS_SECTOR", "")
	t.Setenv("APP_ENV", "development")
	if !Load().ShowProjectsSector() {
		t.Fatal("개발 노트북에서는 사업관리가 보여야 한다")
	}
	t.Setenv("APP_ENV", "production")
	if !Load().ShowProjectsSector() {
		t.Fatal("APP_ENV=production 만으로 로컬 사업관리를 끄면 안 된다")
	}
	t.Setenv("SHOW_PROJECTS_SECTOR", "1")
	if !Load().ShowProjectsSector() {
		t.Fatal("SHOW_PROJECTS_SECTOR=1 이면 연다")
	}
	t.Setenv("APP_ENV", "development")
	t.Setenv("SHOW_PROJECTS_SECTOR", "0")
	if Load().ShowProjectsSector() {
		t.Fatal("SHOW_PROJECTS_SECTOR=0 이면 운영 배포처럼 끈다")
	}
}

func TestShowProjectExampleDataOffInProduction(t *testing.T) {
	t.Setenv("PROJECT_EXAMPLE_DATA", "")
	t.Setenv("APP_ENV", "development")
	if !Load().ShowProjectExampleData() {
		t.Fatal("개발에서는 예시 데이터가 있어야 한다")
	}
	t.Setenv("APP_ENV", "production")
	if Load().ShowProjectExampleData() {
		t.Fatal("운영 서버에는 예시 인명·공정을 올리면 안 된다")
	}
	t.Setenv("APP_ENV", "development")
	t.Setenv("PROJECT_EXAMPLE_DATA", "0")
	if Load().ShowProjectExampleData() {
		t.Fatal("PROJECT_EXAMPLE_DATA=0 이면 끈다")
	}
}
