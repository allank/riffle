package config

import (
	"errors"
	"os"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Top         int
	Format      string
	Pretty      bool
	Relative    bool
	Ext         []string
	Exclude     []string
	Depth       int
	Concurrency int
}

type tomlFile struct {
	Defaults struct {
		Top      int    `toml:"top"`
		Format   string `toml:"format"`
		Pretty   bool   `toml:"pretty"`
		Relative *bool  `toml:"relative"`
	} `toml:"defaults"`
	Index struct {
		Ext         []string `toml:"ext"`
		Exclude     []string `toml:"exclude"`
		Depth       int      `toml:"depth"`
		Concurrency int      `toml:"concurrency"`
	} `toml:"index"`
}

func Defaults() Config {
	return Config{
		Top:         5,
		Format:      "plain",
		Pretty:      false,
		Relative:    true,
		Ext:         []string{".md"},
		Depth:       0,
		Concurrency: 0,
	}
}

func Load(path string) (Config, error) {
	cfg := Defaults()
	var tf tomlFile
	_, err := toml.DecodeFile(path, &tf)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return cfg, err
	}
	if tf.Defaults.Top != 0 {
		cfg.Top = tf.Defaults.Top
	}
	if tf.Defaults.Format != "" {
		cfg.Format = tf.Defaults.Format
	}
	cfg.Pretty = tf.Defaults.Pretty
	if tf.Defaults.Relative != nil {
		cfg.Relative = *tf.Defaults.Relative
	}
	if len(tf.Index.Ext) > 0 {
		cfg.Ext = tf.Index.Ext
	}
	cfg.Exclude = tf.Index.Exclude
	if tf.Index.Depth != 0 {
		cfg.Depth = tf.Index.Depth
	}
	if tf.Index.Concurrency != 0 {
		cfg.Concurrency = tf.Index.Concurrency
	}
	return cfg, nil
}

func DefaultPath() string {
	home, _ := os.UserHomeDir()
	return home + "/.config/riffle/config.toml"
}
