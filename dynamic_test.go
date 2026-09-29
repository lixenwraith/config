package config

import (
	"fmt"
	"path/filepath"
	"runtime"
	"testing"
)

func TestTOMLOnly(t *testing.T) {
	for _, format := range []string{"json", "yaml", "other"} {
		c := New()
		if err := c.SetFileFormat(format); err == nil {
			t.Fatalf("expected an error")
		}
		_, err := NewBuilder().WithTagName(format).Build()
		if err == nil {
			t.Fatalf("expected an error")
		}
		var dst struct{ Value string }
		if err := ScanMap(nil, &dst, format); err == nil {
			t.Fatalf("expected an error")
		}
	}
	for _, tc := range []struct{ ext, content string }{
		{".toml", `value = "toml"`}, {".conf", `value = "toml"`}, {".config", `value = "toml"`},
	} {
		c := New()
		mustNoError(t, c.Register("value", ""))
		path := filepath.Join(t.TempDir(), "config"+tc.ext)
		writeTestFile(t, path, []byte(tc.content), 0600)
		mustNoError(t, c.LoadFile(path))
	}
	for _, tc := range []struct{ ext, content string }{
		{".json", `{"value":"json"}`}, {".yaml", "value: yaml"}, {".conf", `{"value":"json"}`},
	} {
		c := New()
		mustNoError(t, c.Register("value", ""))
		path := filepath.Join(t.TempDir(), "config"+tc.ext)
		writeTestFile(t, path, []byte(tc.content), 0600)
		if err := c.LoadFile(path); err == nil {
			t.Fatalf("expected an error")
		}
	}
}

func TestSecurityOptions(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("PathTraversal", func(t *testing.T) {
		cfg := New()
		cfg.SetSecurityOptions(SecurityOptions{
			PreventPathTraversal: true,
		})

		// Test various malicious paths
		maliciousPaths := []string{
			"../../../etc/passwd",
			"./../etc/passwd",
			"config/../../../etc/passwd",
			filepath.Join("..", "..", "etc", "passwd"),
		}

		for _, malPath := range maliciousPaths {
			err := cfg.LoadFile(malPath)
			if err == nil {
				t.Errorf("expected an error: %s", fmt.Sprintf("Should reject path: %s", malPath))
			}
			checkErrorContains(t, err, "path traversal")
		}

		// Valid paths should work
		validPath := filepath.Join(tmpDir, "config.toml")
		writeTestFile(t, validPath, []byte(`test = "value"`), 0644)
		cfg.Register("test", "")

		err := cfg.LoadFile(validPath)
		if err != nil {
			t.Errorf("unexpected error: %v: %s", err, "Should accept valid absolute path")
		}
	})

	t.Run("FileSizeLimit", func(t *testing.T) {
		cfg := New()
		cfg.SetSecurityOptions(SecurityOptions{
			MaxFileSize: 100, // 100 bytes limit
		})

		// Create large file
		largePath := filepath.Join(tmpDir, "large.toml")
		largeContent := make([]byte, 1024)
		for i := range largeContent {
			largeContent[i] = 'a'
		}
		writeTestFile(t, largePath, largeContent, 0644)

		err := cfg.LoadFile(largePath)
		checkErrorContains(t, err, "exceeds maximum size")
	})

	t.Run("FileOwnership", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("Skipping ownership test on Windows")
		}

		cfg := New()
		cfg.SetSecurityOptions(SecurityOptions{
			EnforceFileOwnership: true,
		})

		// Create file owned by current user (should succeed)
		ownedPath := filepath.Join(tmpDir, "owned.toml")
		writeTestFile(t, ownedPath, []byte(`test = "value"`), 0644)

		cfg.Register("test", "")
		err := cfg.LoadFile(ownedPath)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
}
