package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type State struct {
	Version       int            `json:"version"`
	Nodes         []Node         `json:"nodes"`
	Rules         []Rule         `json:"rules"`
	ACL           []Rule         `json:"acl"`
	Subscriptions []Subscription `json:"subscriptions"`
}

type Node struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Enabled    bool   `json:"enabled"`
	URI        string `json:"uri"`
	ImportedAt string `json:"imported_at"`
	Source     Source `json:"source"`
}

type Source struct {
	Kind string `json:"kind"`
	ID   string `json:"id,omitempty"`
}

type Rule struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Pattern string `json:"pattern"`
	Target  string `json:"target"`
}

type Subscription struct {
	ID          string                  `json:"id"`
	Name        string                  `json:"name"`
	URL         string                  `json:"url"`
	Enabled     bool                    `json:"enabled"`
	CreatedAt   string                  `json:"created_at"`
	Cache       SubscriptionCache       `json:"cache"`
	Enumeration SubscriptionEnumeration `json:"enumeration"`
}

type SubscriptionCache struct {
	LastAttemptAt string `json:"last_attempt_at"`
	LastSuccessAt string `json:"last_success_at"`
	LastError     string `json:"last_error"`
}

type SubscriptionEnumeration struct {
	LastCount     int    `json:"last_count"`
	LastUpdatedAt string `json:"last_updated_at"`
	Method        string `json:"method"`
}

func Empty() State {
	return State{
		Version:       1,
		Nodes:         []Node{},
		Rules:         []Rule{},
		ACL:           []Rule{},
		Subscriptions: []Subscription{},
	}
}

func Ensure(path string) (State, error) {
	st, err := Load(path)
	if err == nil {
		return st, nil
	}
	if !os.IsNotExist(err) {
		return State{}, err
	}
	st = Empty()
	return st, Save(path, st)
}

func Load(path string) (State, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return State{}, err
	}
	var st State
	if err := json.Unmarshal(raw, &st); err != nil {
		return State{}, fmt.Errorf("parse state: %w", err)
	}
	if st.Nodes == nil {
		st.Nodes = []Node{}
	}
	if st.Rules == nil {
		st.Rules = []Rule{}
	}
	if st.ACL == nil {
		st.ACL = []Rule{}
	}
	if st.Subscriptions == nil {
		st.Subscriptions = []Subscription{}
	}
	if st.Version == 0 {
		st.Version = 1
	}
	return st, nil
}

func Save(path string, st State) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return fmt.Errorf("encode state: %w", err)
	}
	raw = append(raw, '\n')
	return os.WriteFile(path, raw, 0o640)
}

func NowISO() string {
	return time.Now().UTC().Format(time.RFC3339)
}
