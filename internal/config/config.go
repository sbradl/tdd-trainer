// Package config loads .tddtrainer.yml.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

const FileName = ".tddtrainer.yml"

type Config struct {
	Preset         string        `yaml:"preset"`
	Build          string        `yaml:"build"`
	Test           TestCmd       `yaml:"test"`
	Results        Results       `yaml:"results"`
	Tests          []string      `yaml:"tests"`
	Sources        []string      `yaml:"sources"`
	Ignore         []string      `yaml:"ignore"`
	TPPOrder       string        `yaml:"tpp_order"`
	SlowRunWarning time.Duration `yaml:"slow_run_warning"`
}

type TestCmd struct {
	Cmd     string `yaml:"cmd"`
	Windows string `yaml:"windows"`
}

type Results struct {
	Format string `yaml:"format"` // junit-xml | go-json | trx
	Path   string `yaml:"path"`   // for file formats
}

// TestCmd returns the test command for goos, honouring per-OS overrides.
func (c Config) TestCmd(goos string) string {
	if goos == "windows" && c.Test.Windows != "" {
		return c.Test.Windows
	}
	return c.Test.Cmd
}

// Load reads FileName from dir. A missing file yields an os.IsNotExist error.
func Load(dir string) (Config, error) {
	data, err := os.ReadFile(filepath.Join(dir, FileName))
	if err != nil {
		return Config{}, err
	}
	var c Config
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil {
		return Config{}, fmt.Errorf("%s: %w", FileName, err)
	}
	if c.Test.Cmd == "" {
		return Config{}, errors.New(FileName + ": test.cmd is required")
	}
	return c, nil
}
