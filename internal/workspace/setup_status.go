package workspace

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

const SetupStatusRunning = "running"

type SetupStatus struct {
	State     string    `json:"state"`
	PID       int       `json:"pid,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

func SetupStatusPath(contextPath string) (string, error) {
	homeDirectory, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	hash := sha1.Sum([]byte(contextPath))
	fileName := hex.EncodeToString(hash[:8]) + ".json"
	return filepath.Join(homeDirectory, ".config", "squirrel", "setup", fileName), nil
}

func ReadSetupStatus(contextPath string) (SetupStatus, error) {
	statusPath, err := SetupStatusPath(contextPath)
	if err != nil {
		return SetupStatus{}, err
	}

	data, err := os.ReadFile(statusPath)
	if os.IsNotExist(err) {
		return SetupStatus{}, nil
	}
	if err != nil {
		return SetupStatus{}, err
	}

	var status SetupStatus
	if err := json.Unmarshal(data, &status); err != nil {
		return SetupStatus{}, err
	}

	if status.State == SetupStatusRunning && status.PID > 0 && !setupProcessExists(status.PID) {
		_ = ClearSetupStatus(contextPath)
		return SetupStatus{}, nil
	}

	return status, nil
}

func WriteSetupStatus(contextPath, state string, pid int) error {
	statusPath, err := SetupStatusPath(contextPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(statusPath), 0o755); err != nil {
		return err
	}

	status := SetupStatus{
		State:     state,
		PID:       pid,
		UpdatedAt: time.Now().UTC(),
	}
	data, err := json.Marshal(status)
	if err != nil {
		return err
	}
	return os.WriteFile(statusPath, data, 0o644)
}

func ClearSetupStatus(contextPath string) error {
	statusPath, err := SetupStatusPath(contextPath)
	if err != nil {
		return err
	}
	if err := os.Remove(statusPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func setupProcessExists(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return process.Signal(syscall.Signal(0)) == nil
}
