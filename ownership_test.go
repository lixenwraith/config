package config

import "testing"

func TestOptionsAreOwned(t *testing.T) {
	opts := DefaultLoadOptions()
	opts.EnvWhitelist = map[string]bool{"value": true}
	c := NewWithOptions(opts)
	opts.Sources[0] = SourceFile
	opts.EnvWhitelist["value"] = false
	if c.GetPrecedence()[0] != SourceCLI || !c.options.EnvWhitelist["value"] {
		t.Fatal("options retained caller-owned containers")
	}
	c.SetLoadOptions(LoadOptions{})
	if err := c.Register("value", 0); err != nil {
		t.Fatal(err)
	}
	if err := c.Set("value", 1); err != nil {
		t.Fatal(err)
	}
}

func TestExportEnvContainerValues(t *testing.T) {
	c := New()
	if err := c.Register("items", []string{"old"}); err != nil {
		t.Fatal(err)
	}
	if err := c.Set("items", []string{"new"}); err != nil {
		t.Fatal(err)
	}
	if got := c.ExportEnv("APP_")["APP_ITEMS"]; got == "" {
		t.Fatal("changed slice was not exported")
	}
}
