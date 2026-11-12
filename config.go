package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

type Account struct {
	Alias       string `json:"alias"`
	Cookie      string `json:"cookie"`
	UserID      int64  `json:"user_id,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
	Username    string `json:"username,omitempty"`
	Valid       bool   `json:"valid"`
}

type Keybinds struct {
	Up      string `json:"up"`
	Down    string `json:"down"`
	Confirm string `json:"confirm"`
	Back    string `json:"back"`
}

type Config struct {
	Accounts            []Account `json:"accounts"`
	ActiveAccount       string    `json:"active_account"`
	RequestDelaySeconds float64   `json:"request_delay_seconds"`
	DeclineFirstServer  bool      `json:"decline_first_server"`
	RecentTargets       []string  `json:"recent_targets"`
	Keybinds            Keybinds  `json:"keybinds"`
	mu                  sync.Mutex
}

func DefaultConfig() *Config {
	return &Config{
		Accounts:            []Account{},
		ActiveAccount:       "",
		RequestDelaySeconds: 1.0,
		DeclineFirstServer:  false,
		RecentTargets:       []string{},
		Keybinds: Keybinds{
			Up:      "w",
			Down:    "s",
			Confirm: "v",
			Back:    "b",
		},
	}
}

func getConfigPath() string {
	if exePath, err := os.Executable(); err == nil {
		localPath := filepath.Join(filepath.Dir(exePath), "config.json")
		if _, err := os.Stat(localPath); err == nil {
			return localPath
		}
	}

	if configDir, err := os.UserConfigDir(); err == nil {
		appDir := filepath.Join(configDir, "rosniper")
		_ = os.MkdirAll(appDir, 0755)
		return filepath.Join(appDir, "config.json")
	}

	return "config.json"
}

func LoadConfig() (*Config, error) {
	path := getConfigPath()

	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		cfg := DefaultConfig()
		if err := cfg.Save(); err != nil {
			return nil, err
		}
		return cfg, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	cfg := DefaultConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	if cfg.RequestDelaySeconds < 0.6 {
		cfg.RequestDelaySeconds = 0.6
	}

	if cfg.Keybinds.Up == "" {
		cfg.Keybinds.Up = "w"
	}
	if cfg.Keybinds.Down == "" {
		cfg.Keybinds.Down = "s"
	}
	if cfg.Keybinds.Confirm == "" || cfg.Keybinds.Confirm == "enter" {
		cfg.Keybinds.Confirm = "v"
	}
	if cfg.Keybinds.Back == "" || cfg.Keybinds.Back == "shift" {
		cfg.Keybinds.Back = "b"
	}

	return cfg, nil
}

func (c *Config) Save() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(getConfigPath(), data, 0600)
}

func (c *Config) GetActiveCookie() string {
	for _, acc := range c.Accounts {
		if acc.Alias == c.ActiveAccount {
			return acc.Cookie
		}
	}
	if len(c.Accounts) > 0 {
		return c.Accounts[0].Cookie
	}
	return ""
}

func (c *Config) AddRecentTarget(target string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for i, t := range c.RecentTargets {
		if t == target {
			c.RecentTargets = append(c.RecentTargets[:i], c.RecentTargets[i+1:]...)
			break
		}
	}
	c.RecentTargets = append([]string{target}, c.RecentTargets...)
	if len(c.RecentTargets) > 10 {
		c.RecentTargets = c.RecentTargets[:10]
	}
	_ = c.Save()
}
