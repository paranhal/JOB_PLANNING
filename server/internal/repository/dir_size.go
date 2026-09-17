package repository

import (
	"io/fs"
	"os"
	"path/filepath"
)

func DirSize(root string) (bytes int64, files int) {
	root = filepath.Clean(root)
	if root == "" || root == "." {
		return 0, 0
	}
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return nil
		}
		info, e := d.Info()
		if e != nil {
			return nil
		}
		bytes += info.Size()
		files++
		return nil
	})
	return bytes, files
}

func DirExists(root string) bool {
	st, err := os.Stat(root)
	return err == nil && st.IsDir()
}
