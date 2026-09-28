package config

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestTOMLOnly(t *testing.T) {
	for _, format := range []string{"json", "yaml", "other"} {
		c := New()
		require.Error(t, c.SetFileFormat(format))
		_, err := NewBuilder().WithTagName(format).Build()
		require.Error(t, err)
		var dst struct{ Value string }
		require.Error(t, ScanMap(nil, &dst, format))
	}
	for _, tc := range []struct{ ext, content string }{
		{".toml", `value = "toml"`}, {".conf", `value = "toml"`}, {".config", `value = "toml"`},
	} {
		c := New()
		require.NoError(t, c.Register("value", ""))
		path := filepath.Join(t.TempDir(), "config"+tc.ext)
		require.NoError(t, os.WriteFile(path, []byte(tc.content), 0600))
		require.NoError(t, c.LoadFile(path))
	}
	for _, tc := range []struct{ ext, content string }{
		{".json", `{"value":"json"}`}, {".yaml", "value: yaml"}, {".conf", `{"value":"json"}`},
	} {
		c := New()
		require.NoError(t, c.Register("value", ""))
		path := filepath.Join(t.TempDir(), "config"+tc.ext)
		require.NoError(t, os.WriteFile(path, []byte(tc.content), 0600))
		require.Error(t, c.LoadFile(path))
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
			assert.Error(t, err, "Should reject path: %s", malPath)
			assert.Contains(t, err.Error(), "path traversal")
		}

		// Valid paths should work
		validPath := filepath.Join(tmpDir, "config.toml")
		os.WriteFile(validPath, []byte(`test = "value"`), 0644)
		cfg.Register("test", "")

		err := cfg.LoadFile(validPath)
		assert.NoError(t, err, "Should accept valid absolute path")
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
		require.NoError(t, os.WriteFile(largePath, largeContent, 0644))

		err := cfg.LoadFile(largePath)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "exceeds maximum size")
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
		require.NoError(t, os.WriteFile(ownedPath, []byte(`test = "value"`), 0644))

		cfg.Register("test", "")
		err := cfg.LoadFile(ownedPath)
		assert.NoError(t, err)
	})
}

// waitForWatchingState waits for watcher state, preventing race conditions of goroutine start and test check
func waitForWatchingState(t *testing.T, cfg *Config, expected bool, msgAndArgs ...any) {
	t.Helper()
	require.Eventually(t, func() bool {
		return cfg.IsWatching() == expected
	}, testEventuallyTimeout, 2*SpinWaitInterval, msgAndArgs...)
}
