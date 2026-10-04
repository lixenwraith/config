package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fuzzTarget struct {
	Name  string            `toml:"name"`
	Port  int64             `toml:"port"`
	Ratio float64           `toml:"ratio"`
	On    bool              `toml:"on"`
	Tags  []string          `toml:"tags"`
	IDs   []uint16          `toml:"ids"`
	Meta  map[string]string `toml:"meta"`
	Sub   struct {
		Level string `toml:"level"`
	} `toml:"sub"`
}

// Arguments never panic the parser, and what a load accepts decodes
func FuzzLoadCLI(f *testing.F) {
	for _, s := range []string{"--name=x", "--port\x009000", "--on\x00--tags=a,b", "-q\x00x\x00--\x00--name", "--sub.level\x00--on", "--ids=1,65536"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		cfg, err := NewBuilder().WithTarget(&fuzzTarget{}).WithArgs(strings.Split(s, "\x00")).Build()
		if err != nil {
			return
		}
		if _, err := cfg.AsStruct(); err != nil {
			t.Fatalf("accepted %q, then: %v", s, err)
		}
	})
}

// File contents never panic the loader, and what LoadFile accepts decodes
func FuzzLoadFile(f *testing.F) {
	for _, s := range []string{"name = \"x\"\nport = 1", "tags = \"a,b\"\n[sub]\nlevel = \"debug\"", "ids = [1, 2]\n[meta]\nk = \"v\"", "on = true # c\r\n"} {
		f.Add([]byte(s))
	}
	// One file and one Config per worker: each LoadFile replaces the file source
	path := filepath.Join(f.TempDir(), "c.toml")
	cfg, err := NewBuilder().WithTarget(&fuzzTarget{}).Build()
	if err != nil {
		f.Fatal(err)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		if cfg.LoadFile(path) != nil {
			return
		}
		if _, err := cfg.AsStruct(); err != nil {
			t.Fatalf("accepted %q, then: %v", data, err)
		}
	})
}
