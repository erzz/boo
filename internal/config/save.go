package config

import (
	"errors"
	"fmt"
	"os"

	"sigs.k8s.io/yaml"

	"github.com/erzz/boo/internal/state"
)

// SetUITheme writes ui.theme=name into the config file at path,
// preserving every other key. Creates the file if it doesn't exist.
//
// Round-trip caveat: this loads the YAML into the typed Config struct
// and re-marshals. Any comments or formatting in the original file are
// LOST. boo doesn't write comments itself, so for files boo wrote
// originally this is a no-op; for hand-edited files with comments this
// is rude. Acceptable for now (pre-release, used only by the picker's
// theme cycler). If we ever expose general config-from-TUI editing,
// swap in a comment-preserving YAML library here.
func SetUITheme(path, name string) error {
	if path == "" {
		return errors.New("config path is empty")
	}

	cfg, err := loadForWrite(path)
	if err != nil {
		return err
	}

	cfg.UI.Theme = &name

	out, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := state.WriteAtomic(path, out); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// loadForWrite returns the parsed config at path, or a zero Config if
// the file doesn't exist. Unlike Load, it does NOT apply factory
// defaults — we want to preserve the "unset" state of every field the
// user hasn't touched, so the rewrite doesn't materialise factory
// values into the file.
func loadForWrite(path string) (Config, error) {
	var cfg Config
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return cfg, fmt.Errorf("read %s: %w", path, err)
	}
	if len(data) == 0 {
		return cfg, nil
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse %s: %w", path, err)
	}
	return cfg, nil
}
