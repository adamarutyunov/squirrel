package agent

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

const (
	StatusIdle     = "idle"
	StatusThinking = "thinking"
	StatusDone     = "done"
	StatusUnknown  = ""
)

type Status struct {
	State     string    `json:"state"`
	UpdatedAt time.Time `json:"updated_at"`
}

func StatusPath(contextPath string) (string, error) {
	homeDirectory, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	hash := sha1.Sum([]byte(contextPath))
	fileName := hex.EncodeToString(hash[:8]) + ".json"
	return filepath.Join(homeDirectory, ".config", "squirrel", "agents", fileName), nil
}

func ReadStatus(contextPath string) (Status, error) {
	statusPath, err := StatusPath(contextPath)
	if err != nil {
		return Status{}, err
	}

	data, err := os.ReadFile(statusPath)
	if os.IsNotExist(err) {
		return Status{}, nil
	}
	if err != nil {
		return Status{}, err
	}

	var status Status
	if err := json.Unmarshal(data, &status); err != nil {
		return Status{}, err
	}
	return status, nil
}

func WriteStatus(contextPath, state string) error {
	statusPath, err := StatusPath(contextPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(statusPath), 0o755); err != nil {
		return err
	}

	status := Status{
		State:     state,
		UpdatedAt: time.Now().UTC(),
	}
	data, err := json.Marshal(status)
	if err != nil {
		return err
	}
	return os.WriteFile(statusPath, data, 0o644)
}

func SessionIDPath(contextPath string) (string, error) {
	homeDirectory, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	hash := sha1.Sum([]byte(contextPath))
	fileName := hex.EncodeToString(hash[:8]) + ".session"
	return filepath.Join(homeDirectory, ".config", "squirrel", "agents", fileName), nil
}

func WriteSessionID(contextPath, sessionID string) error {
	sessionIDPath, err := SessionIDPath(contextPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(sessionIDPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(sessionIDPath, []byte(sessionID), 0o644)
}

func ReadSessionID(contextPath string) (string, error) {
	sessionIDPath, err := SessionIDPath(contextPath)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(sessionIDPath)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func RemoveStatus(contextPath string) error {
	statusPath, err := StatusPath(contextPath)
	if err != nil {
		return err
	}
	if err := os.Remove(statusPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
