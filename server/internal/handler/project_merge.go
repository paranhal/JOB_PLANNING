package handler

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/backup"
	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func (h *ProjectHandler) MergeForm(c echo.Context) error {
	if currentRole(c) != model.RoleVisionAdmin {
		return echo.ErrForbidden
	}
	keep, err := h.repo.Get(c.Param("id"))
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/projects?err=notfound")
	}
	others, _ := h.repo.ListOthers(keep.ProjectID)
	dropID := strings.TrimSpace(c.QueryParam("drop_id"))
	var preview *repository.ProjectMergePreview
	if dropID != "" {
		p, err := h.repo.MergePreview(keep.ProjectID, dropID)
		if err == nil {
			preview = &p
		}
	}
	return c.Render(http.StatusOK, "project/merge.html", map[string]interface{}{
		"Title": "사업 합치기", "Active": NavProjects,
		"Keep": keep, "Others": others, "DropID": dropID, "Preview": preview,
		"FlashErr": c.QueryParam("err"), "FlashOK": c.QueryParam("ok"),
		"MergeID": c.QueryParam("merge_id"),
	})
}

func (h *ProjectHandler) MergeSave(c echo.Context) error {
	if currentRole(c) != model.RoleVisionAdmin {
		return echo.ErrForbidden
	}
	keepID := c.Param("id")
	dropID := strings.TrimSpace(c.FormValue("drop_id"))
	if dropID == "" {
		return c.Redirect(http.StatusSeeOther, "/projects/"+keepID+"/merge?err=drop")
	}
	backupName, err := h.runMergeBackup()
	if err != nil {
		log.Printf("project merge backup: %v", err)
		return c.Redirect(http.StatusSeeOther, "/projects/"+keepID+"/merge?drop_id="+dropID+"&err=backup")
	}
	mergeID, err := h.repo.MergeProjects(keepID, dropID, backupName)
	if err != nil {
		log.Printf("project merge: %v", err)
		return c.Redirect(http.StatusSeeOther, "/projects/"+keepID+"/merge?drop_id="+dropID+"&err=merge")
	}
	return c.Redirect(http.StatusSeeOther, "/projects/"+keepID+"/merge?ok=merged&merge_id="+mergeID+"&drop_id="+dropID)
}

func (h *ProjectHandler) MergeUndo(c echo.Context) error {
	if currentRole(c) != model.RoleVisionAdmin {
		return echo.ErrForbidden
	}
	keepID := c.Param("id")
	mergeID := strings.TrimSpace(c.FormValue("merge_id"))
	if mergeID == "" {
		dropID := strings.TrimSpace(c.FormValue("drop_id"))
		var err error
		mergeID, err = h.repo.LatestMergeFor(keepID, dropID)
		if err != nil {
			return c.Redirect(http.StatusSeeOther, "/projects/"+keepID+"/merge?err=nomerge")
		}
	}
	if err := h.repo.UndoMerge(mergeID); err != nil {
		log.Printf("project merge undo: %v", err)
		return c.Redirect(http.StatusSeeOther, "/projects/"+keepID+"/merge?err=undo")
	}
	return c.Redirect(http.StatusSeeOther, "/projects/"+keepID+"?ok=merge_undone")
}

func (h *ProjectHandler) runMergeBackup() (string, error) {
	if h.backup.DB != nil && strings.TrimSpace(h.backup.DataDir) != "" {
		name, err := backup.Snapshot(h.backup, backup.KindManual)
		if err == nil {
			if strings.TrimSpace(os.Getenv("DATA_DIR")) != "" {
				h.tryBackupDBScript()
			}
			return name, nil
		}
		log.Printf("project merge Snapshot: %v", err)
	}
	if name := h.tryBackupDBScript(); name != "" {
		return name, nil
	}
	return "", fmt.Errorf("합치기 전 백업에 실패했습니다")
}

func (h *ProjectHandler) tryBackupDBScript() string {
	dataDir := strings.TrimSpace(os.Getenv("DATA_DIR"))
	if dataDir == "" && strings.TrimSpace(h.backup.DataDir) != "" {
		dataDir = h.backup.DataDir
	}
	if dataDir == "" {
		return ""
	}
	candidates := []string{
		filepath.Join("deploy", "backup-db.sh"),
		filepath.Join("..", "deploy", "backup-db.sh"),
	}
	for _, script := range candidates {
		if st, err := os.Stat(script); err != nil || st.IsDir() {
			continue
		}
		cmd := exec.Command("/bin/sh", script, "merge")
		cmd.Env = append(os.Environ(), "DATA_DIR="+dataDir)
		out, err := cmd.CombinedOutput()
		if err != nil {
			log.Printf("backup-db.sh: %v %s", err, out)
			continue
		}
		return strings.TrimSpace(string(out))
	}
	return ""
}
