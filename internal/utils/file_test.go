package utils

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteFileAtomicCreatesBackupOfPreviousVersion(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "state.json")
	if err := WriteFileAtomic(path, []byte(`{"v":1}`)); err != nil {
		t.Fatalf("first write failed: %v", err)
	}
	if err := WriteFileAtomic(path, []byte(`{"v":2}`)); err != nil {
		t.Fatalf("second write failed: %v", err)
	}

	backup, err := os.ReadFile(BackupPath(path))
	if err != nil {
		t.Fatalf("failed to read backup: %v", err)
	}
	if string(backup) != `{"v":1}` {
		t.Fatalf("unexpected backup contents: %s", string(backup))
	}
}

func TestReadJSONFileWithBackupFallsBackToBackup(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	if err := os.WriteFile(BackupPath(path), []byte(`{"value":42}`), 0644); err != nil {
		t.Fatalf("failed to seed backup: %v", err)
	}
	if err := os.WriteFile(path, []byte(`{"value":`), 0644); err != nil {
		t.Fatalf("failed to seed primary: %v", err)
	}

	var got struct {
		Value int `json:"value"`
	}
	source, err := ReadJSONFileWithBackup(path, &got)
	if err != nil {
		t.Fatalf("ReadJSONFileWithBackup failed: %v", err)
	}
	if source != BackupPath(path) {
		t.Fatalf("expected backup source, got %s", source)
	}
	if got.Value != 42 {
		t.Fatalf("unexpected value: %d", got.Value)
	}
}
