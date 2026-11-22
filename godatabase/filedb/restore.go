package filedb

import (
	"fmt"
	"os"
)

func recoverRestoreFiles(filePath string) error {
	oldPath := filePath + ".restore-old"
	_, mainErr := os.Stat(filePath)
	_, oldErr := os.Stat(oldPath)
	if mainErr != nil && !os.IsNotExist(mainErr) {
		return fmt.Errorf("filedb: inspect restored database: %w", mainErr)
	}
	if oldErr == nil {
		if os.IsNotExist(mainErr) {
			if err := os.Rename(oldPath, filePath); err != nil {
				return fmt.Errorf("filedb: recover previous database: %w", err)
			}
		} else if err := os.Remove(oldPath); err != nil {
			return fmt.Errorf("filedb: clean restore backup: %w", err)
		}
	}
	if err := os.Remove(filePath + ".restore-tmp"); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("filedb: clean restore staging: %w", err)
	}
	return nil
}
