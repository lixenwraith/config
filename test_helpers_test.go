package config

import (
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

// Container and interface comparisons must retain Go types and nil/empty distinctions.
func checkEqual(t testing.TB, got, want any, context ...string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		prefix := ""
		if len(context) > 0 {
			prefix = strings.Join(context, "; ") + ": "
		}
		t.Errorf("%sgot %#v (%T), want %#v (%T)", prefix, got, got, want, want)
	}
}

func mustNoError(t testing.TB, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func checkErrorContains(t testing.TB, err error, fragment string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), fragment) {
		t.Errorf("error = %v, want text containing %q", err, fragment)
	}
}

func mustPanic(t testing.TB, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatal("expected a panic")
		}
	}()
	fn()
}

func writeTestFile(t testing.TB, path string, data []byte, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, data, mode); err != nil {
		t.Fatalf("write fixture %q: %v", path, err)
	}
}

// File-backed capture avoids pipe capacity limits and restores stdout even if
// the tested operation calls Fatal. Tests using process globals run serially.
func captureStdout(t testing.TB, fn func()) string {
	t.Helper()
	file, err := os.CreateTemp(t.TempDir(), "stdout-*")
	mustNoError(t, err)
	previous := os.Stdout
	t.Cleanup(func() { os.Stdout = previous; _ = file.Close() })
	os.Stdout = file
	fn()
	os.Stdout = previous
	mustNoError(t, file.Close())
	data, err := os.ReadFile(file.Name())
	mustNoError(t, err)
	return string(data)
}

func checkWatchingState(t testing.TB, cfg *Config, expected bool, context ...string) {
	t.Helper()
	// Starting and stopping publish their state synchronously.
	checkEqual(t, cfg.IsWatching(), expected, context...)
}

func receiveWatchEvent(t testing.TB, changes <-chan string) string {
	t.Helper()
	select {
	case event, ok := <-changes:
		if !ok {
			t.Fatal("watch channel closed before the expected event")
		}
		return event
	case <-time.After(testWatchTimeout):
		t.Fatal("timed out waiting for a watch event")
		return ""
	}
}
