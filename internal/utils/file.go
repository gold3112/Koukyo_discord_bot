package utils

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
)

const backupSuffix = ".bak"

func BackupPath(path string) string {
	return path + backupSuffix
}

func ReadJSONFileWithBackup(path string, dest interface{}) (string, error) {
	source, err := readJSONFile(path, dest)
	if err == nil {
		return source, nil
	}

	backupPath := BackupPath(path)
	backupSource, backupErr := readJSONFile(backupPath, dest)
	if backupErr == nil {
		log.Printf("Recovered JSON from backup: primary=%s backup=%s err=%v", path, backupPath, err)
		return backupSource, nil
	}

	if errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	return "", err
}

func readJSONFile(path string, dest interface{}) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if len(data) == 0 {
		return path, nil
	}
	if err := json.Unmarshal(data, dest); err != nil {
		return "", err
	}
	return path, nil
}

// WriteFileAtomic writes payload to filename in a directory in an atomic-like manner.
// It writes to a temporary file first and then renames it to the destination.
func WriteFileAtomic(path string, payload []byte) error {
	dir := filepath.Dir(path)
	filename := filepath.Base(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, filename+".tmp.*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	tmpClosed := false
	existingData, existingErr := os.ReadFile(path)
	hasExisting := existingErr == nil

	// Ensure cleanup in case of error
	success := false
	defer func() {
		if !success {
			if !tmpClosed {
				_ = tmp.Close()
			}
			_ = os.Remove(tmpName)
		}
	}()

	if _, err := tmp.Write(payload); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	tmpClosed = true

	if err := os.Rename(tmpName, path); err != nil {
		// On Windows, os.Rename can fail if destination exists.
		// Move the existing file aside first, then swap in the new file.
		if _, statErr := os.Stat(path); statErr != nil {
			return fmt.Errorf("failed to rename temp file: %w", err)
		}
		backupPath := path + ".bak.tmp"
		_ = os.Remove(backupPath)
		if backupErr := os.Rename(path, backupPath); backupErr != nil {
			return fmt.Errorf("failed to backup existing file: %w (original rename err: %v)", backupErr, err)
		}
		if renameErr := os.Rename(tmpName, path); renameErr != nil {
			_ = os.Rename(backupPath, path)
			return fmt.Errorf("failed to rename temp file after backup: %w", renameErr)
		}
		_ = os.Remove(backupPath)
	}

	success = true
	if hasExisting && len(existingData) > 0 {
		backupPath := BackupPath(path)
		if err := os.WriteFile(backupPath, existingData, 0644); err != nil {
			log.Printf("failed to update backup file %s: %v", backupPath, err)
		}
	}
	return nil
}
