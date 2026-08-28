package handler

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

type WorkHandler struct {
	repo     *repository.WorkBoardRepo
	asRepo   *repository.ASRepo
	mntRepo  *repository.MaintenanceRepo
	wbRepo   *repository.WBRepo
	userRepo *repository.UserRepo
}

func NewWorkHandler(
	repo *repository.WorkBoardRepo,
	asRepo *repository.ASRepo,
	mntRepo *repository.MaintenanceRepo,
	wbRepo *repository.WBRepo,
	userRepo *repository.UserRepo,
) *WorkHandler {
	return &WorkHandler{repo: repo, asRepo: asRepo, mntRepo: mntRepo, wbRepo: wbRepo, userRepo: userRepo}
}

func (h *WorkHandler) List(c echo.Context) error {
	bucket := strings.TrimSpace(c.QueryParam("bucket"))
	if bucket == "" {
		bucket = model.WorkBucketOpen
	}
	role := currentRole(c)
	uid := currentUserID(c)
	keys := assigneeKeys(c)

	mineParam := c.QueryParam("mine")
	mineUID, mineKeys := "", []string(nil)
	scopeAll := false
	if bucket == model.WorkBucketUnassigned {
		scopeAll = true
	} else if role == model.RoleTech {
		if mineParam == "0" {
			scopeAll = true
		} else {
			mineUID, mineKeys = uid, keys
		}
	} else if mineParam == "1" {
		mineUID, mineKeys = uid, keys
	}

	items, err := h.repo.ListBucket(bucket, mineUID, mineKeys, 300)
	if err != nil {
		return err
	}

	showAssignee := role == model.RoleAdmin || role == model.RoleOffice || scopeAll
	title := workBucketTitle(bucket)
	return c.Render(http.StatusOK, "work/list.html", map[string]interface{}{
		"Title":        title,
		"Active":       NavWork,
		"Bucket":       bucket,
		"BucketLabel":  title,
		"Items":        items,
		"Total":        len(items),
		"ShowAssignee": showAssignee,
		"Role":         role,
		"Mine":         mineUID != "",
		"ScopeNote":    workScopeNote(role, scopeAll, bucket),
	})
}

func workBucketTitle(bucket string) string {
	switch bucket {
	case model.WorkBucketToday:
		return "오늘 예정 업무"
	case model.WorkBucketDelayed:
		return "지연 업무"
	case model.WorkBucketCompletedToday:
		return "오늘 완료"
	case model.WorkBucketSchedulePending:
		return "방문 미확정"
	case model.WorkBucketUnassigned:
		return "담당자 미배정"
	default:
		return "전체 업무"
	}
}

func workScopeNote(role string, scopeAll bool, bucket string) string {
	if bucket == model.WorkBucketUnassigned {
		return "전체 접수 기준(담당자 없는 건)"
	}
	if role == model.RoleTech && !scopeAll {
		return "내 배정 업무 기준"
	}
	return "전체 업무 기준"
}

func workListURL(bucket string, mine bool, role string) string {
	v := url.Values{}
	v.Set("bucket", bucket)
	if role == model.RoleTech {
		if mine {
			v.Set("mine", "1")
		} else {
			v.Set("mine", "0")
		}
	}
	return "/work?" + v.Encode()
}

func planUnplannedURL(mine bool, role, kind string) string {
	v := url.Values{}
	if role == model.RoleTech {
		if mine {
			v.Set("mine", "1")
		} else {
			v.Set("mine", "0")
		}
	}
	if kind != "" {
		v.Set("kind", kind)
	}
	s := v.Encode()
	if s == "" {
		return "/plan/unplanned"
	}
	return "/plan/unplanned?" + s
}
