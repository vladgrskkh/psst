// Package config loads psst configuration and theme from a TOML file.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

const (
	appName = "psst"

	defaultSyncIntervalSeconds    = 30
	defaultPushDebounceMs         = 500
	defaultTombstoneRetentionDays = 30

	themeBg     = "#1e1e2e"
	themeAccent = "#89b4fa"
	themeDanger = "#f38ba8"
)

type GeneralConfig struct {
	DefaultList            string `toml:"default_list"`
	SyncIntervalSeconds    int    `toml:"sync_interval_seconds"`
	PushDebounceMs         int    `toml:"push_debounce_ms"`
	TombstoneRetentionDays int    `toml:"tombstone_retention_days"`
	DefaultTime            string `toml:"default_time"`
	DBPath                 string `toml:"db_path"`
	LogLevel               string `toml:"log_level"`
}

type ThemeConfig struct {
	Bg       string `toml:"bg"`
	Surface  string `toml:"surface"`
	Border   string `toml:"border"`
	Text     string `toml:"text"`
	Muted    string `toml:"muted"`
	Accent   string `toml:"accent"`
	Success  string `toml:"success"`
	Warning  string `toml:"warning"`
	Danger   string `toml:"danger"`
	Overdue  string `toml:"overdue"`
	Today    string `toml:"today"`
	Tomorrow string `toml:"tomorrow"`
	Later    string `toml:"later"`
	Cursor   string `toml:"cursor"`
	Progress string `toml:"progress"`
}

type KeysConfig struct {
	Up         []string `toml:"up"`
	Down       []string `toml:"down"`
	ToggleDone []string `toml:"toggle_done"`
	New        []string `toml:"new"`
	Edit       []string `toml:"edit"`
	Delete     []string `toml:"delete"`
	Confirm    []string `toml:"confirm"`
	Cancel     []string `toml:"cancel"`
	ToggleView []string `toml:"toggle_view"`
	Filter     []string `toml:"filter"`
	SyncNow    []string `toml:"sync_now"`
	Quit       []string `toml:"quit"`
}

type Config struct {
	General  GeneralConfig `toml:"general"`
	Theme    ThemeConfig   `toml:"theme"`
	Keys     KeysConfig    `toml:"keys"`
	Warnings []string      `toml:"-"`
}

// Default returns the config with all built-in defaults.
func Default() *Config {
	return &Config{
		General: GeneralConfig{
			DefaultList:            appName,
			SyncIntervalSeconds:    defaultSyncIntervalSeconds,
			PushDebounceMs:         defaultPushDebounceMs,
			TombstoneRetentionDays: defaultTombstoneRetentionDays,
			DefaultTime:            "09:00",
			DBPath:                 "",
			LogLevel:               "info",
		},
		Theme: ThemeConfig{
			Bg: themeBg, Surface: "#313244", Border: "#45475a", Text: "#cdd6f4",
			Muted: "#6c7086", Accent: themeAccent, Success: "#a6e3a1", Warning: "#f9e2af",
			Danger: themeDanger, Overdue: themeDanger, Today: "#fab387", Tomorrow: themeAccent,
			Later: "#6c7086", Cursor: "#cba6f7", Progress: "#cba6f7",
		},
		Keys: KeysConfig{
			Up: []string{"k", "up"}, Down: []string{"j", "down"},
			ToggleDone: []string{"space", "x"}, New: []string{"n"}, Edit: []string{"e"},
			Delete: []string{"d"}, Confirm: []string{"y"}, Cancel: []string{"esc"},
			ToggleView: []string{"tab"}, Filter: []string{"/"}, SyncNow: []string{"s"},
			Quit: []string{"q", "ctrl+c"},
		},
	}
}

// Load reads the TOML file at path on top of Default. A missing file returns
// the defaults. Unknown keys are collected in Warnings.
func Load(path string) (*Config, error) {
	cfg := Default()
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	md, err := toml.DecodeFile(path, cfg)
	if err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	for _, k := range md.Undecoded() {
		cfg.Warnings = append(cfg.Warnings, fmt.Sprintf("unknown config key: %s", k.String()))
	}
	return cfg, nil
}

// StateDir returns ~/.local/state/psst, creating it if needed.
func StateDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	dir := filepath.Join(home, ".local", "state", appName)
	err = os.MkdirAll(dir, 0o700)
	if err != nil {
		return "", fmt.Errorf("create state dir: %w", err)
	}
	return dir, nil
}

func DefaultDBPath() (string, error) {
	dir, err := StateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "psst.db"), nil
}

func DefaultLogPath() (string, error) {
	dir, err := StateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "psst.log"), nil
}
