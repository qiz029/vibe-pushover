package dedupe

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	DefaultTTL = 3 * time.Second
	lockTries  = 20
)

type Status uint8

const (
	StatusAcquired Status = iota
	StatusPending
	StatusDelivered
)

type Reservation struct {
	Fingerprint string
	Token       string
}

type Result struct {
	Status      Status
	Reservation Reservation
}

type Store struct {
	Path string
	Now  func() time.Time
	TTL  time.Duration
}

type entry struct {
	Timestamp int64  `json:"timestamp"`
	State     string `json:"state"`
	Token     string `json:"token"`
}

type state struct {
	Entries map[string]entry `json:"entries"`
}

func DefaultPath() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("find user cache directory: %w", err)
	}
	return filepath.Join(dir, "vibe-pushover", "dedupe.json"), nil
}

// Reserve atomically claims a notification fingerprint, or reports that
// another process is delivering it or has already delivered it recently.
func (s Store) Reserve(fingerprint string) (Result, error) {
	if s.Path == "" || fingerprint == "" {
		return Result{Status: StatusAcquired}, nil
	}
	token, err := randomToken()
	if err != nil {
		return Result{}, fmt.Errorf("create dedupe reservation token: %w", err)
	}
	now := s.now()
	result := Result{}
	err = s.withLock(func() error {
		current, err := readState(s.Path)
		if err != nil {
			return err
		}
		cutoff := now.Add(-s.ttl()).UnixMilli()
		futureCutoff := now.Add(s.ttl()).UnixMilli()
		for key, currentEntry := range current.Entries {
			if currentEntry.Timestamp < cutoff || currentEntry.Timestamp > futureCutoff {
				delete(current.Entries, key)
			}
		}
		if existing, ok := current.Entries[fingerprint]; ok {
			if existing.State == "sent" {
				result.Status = StatusDelivered
			} else {
				result.Status = StatusPending
			}
			return nil
		}
		reservation := Reservation{Fingerprint: fingerprint, Token: token}
		current.Entries[fingerprint] = entry{Timestamp: now.UnixMilli(), State: "pending", Token: token}
		if err := writeState(s.Path, current); err != nil {
			return err
		}
		result = Result{Status: StatusAcquired, Reservation: reservation}
		return nil
	})
	return result, err
}

// Commit marks a successfully delivered reservation. A stale owner cannot
// commit over a newer reservation because the ownership token must match.
func (s Store) Commit(reservation Reservation) error {
	if s.Path == "" || reservation.Fingerprint == "" || reservation.Token == "" {
		return nil
	}
	return s.withLock(func() error {
		current, err := readState(s.Path)
		if err != nil {
			return err
		}
		currentEntry, ok := current.Entries[reservation.Fingerprint]
		if !ok || currentEntry.Token != reservation.Token {
			return nil
		}
		currentEntry.State = "sent"
		currentEntry.Timestamp = s.now().UnixMilli()
		current.Entries[reservation.Fingerprint] = currentEntry
		return writeState(s.Path, current)
	})
}

// Release removes a failed reservation only when the ownership token still
// matches, allowing another process to retry without deleting a newer owner.
func (s Store) Release(reservation Reservation) error {
	if s.Path == "" || reservation.Fingerprint == "" || reservation.Token == "" {
		return nil
	}
	return s.withLock(func() error {
		current, err := readState(s.Path)
		if err != nil {
			return err
		}
		currentEntry, ok := current.Entries[reservation.Fingerprint]
		if !ok || currentEntry.Token != reservation.Token {
			return nil
		}
		delete(current.Entries, reservation.Fingerprint)
		return writeState(s.Path, current)
	})
}

func (s Store) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s Store) ttl() time.Duration {
	if s.TTL > 0 {
		return s.TTL
	}
	return DefaultTTL
}

func (s Store) withLock(action func() error) error {
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o700); err != nil {
		return fmt.Errorf("create dedupe directory: %w", err)
	}
	lockPath := s.Path + ".lock"
	for attempt := 0; attempt < lockTries; attempt++ {
		err := os.Mkdir(lockPath, 0o700)
		if err == nil {
			defer os.Remove(lockPath)
			return action()
		}
		if !errors.Is(err, os.ErrExist) {
			return fmt.Errorf("create dedupe lock: %w", err)
		}
		time.Sleep(5 * time.Millisecond)
	}
	// Never remove or take over an existing lock: doing so safely requires an
	// OS advisory lock. Dedupe callers fail open when this error is returned.
	return errors.New("timed out acquiring dedupe lock")
}

func randomToken() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}

func readState(path string) (state, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return state{Entries: map[string]entry{}}, nil
	}
	if err != nil {
		return state{}, fmt.Errorf("read dedupe state: %w", err)
	}
	var current state
	if err := json.Unmarshal(data, &current); err != nil {
		return state{}, fmt.Errorf("parse dedupe state: %w", err)
	}
	if current.Entries == nil {
		current.Entries = map[string]entry{}
	}
	return current, nil
}

func writeState(path string, current state) error {
	data, err := json.Marshal(current)
	if err != nil {
		return fmt.Errorf("encode dedupe state: %w", err)
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(path), ".dedupe-*")
	if err != nil {
		return fmt.Errorf("create temporary dedupe state: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("set dedupe state permissions: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write dedupe state: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close dedupe state: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		// Windows cannot replace an existing destination with os.Rename. The
		// store lock excludes readers while this best-effort fallback runs.
		if removeErr := os.Remove(path); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			return fmt.Errorf("replace dedupe state: %w", err)
		}
		if retryErr := os.Rename(tmpPath, path); retryErr != nil {
			return fmt.Errorf("replace dedupe state: %w", retryErr)
		}
	}
	return nil
}
