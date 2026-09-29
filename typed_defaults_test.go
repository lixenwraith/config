package config

import "testing"

func TestTypedStructDefaultsAndSnapshots(t *testing.T) {
	type plugin struct {
		Limit uint64         `toml:"limit"`
		Data  map[string]any `toml:"data"`
	}
	type settings struct {
		Plugins []plugin           `toml:"plugins"`
		ByName  map[string]*plugin `toml:"by_name"`
		Fixed   [1]plugin          `toml:"fixed"`
	}
	item := plugin{Limit: ^uint64(0), Data: map[string]any{"tags": []string{"original"}}}
	initial := settings{Plugins: []plugin{item}, ByName: map[string]*plugin{"one": &item}, Fixed: [1]plugin{item}}
	cfg, err := NewBuilder().WithTarget(&initial).WithArgs(nil).Build()
	if err != nil {
		t.Fatal(err)
	}
	value, err := cfg.AsStruct()
	if err != nil {
		t.Fatal(err)
	}
	first := value.(*settings)
	initial.Plugins[0].Data["tags"].([]string)[0] = "caller"
	first.Plugins[0].Data["tags"].([]string)[0] = "snapshot"
	first.ByName["one"].Limit = 0
	first.Fixed[0].Data["tags"] = nil
	value, err = cfg.AsStruct()
	if err != nil {
		t.Fatal(err)
	}
	next := value.(*settings)
	for _, got := range []plugin{next.Plugins[0], *next.ByName["one"], next.Fixed[0]} {
		if got.Limit != ^uint64(0) || got.Data["tags"].([]string)[0] != "original" {
			t.Fatalf("native struct was truncated or aliased: %+v", got)
		}
	}
}

func TestTypedStructDecodeStillRejectsCycles(t *testing.T) {
	type node struct {
		Next *node `toml:"next"`
	}
	cyclic := &node{}
	cyclic.Next = cyclic
	target := struct {
		Nodes []node `toml:"nodes"`
	}{Nodes: []node{{}}}
	if err := ScanMap(map[string]any{"nodes": []node{*cyclic}}, &target); err == nil {
		t.Fatal("native struct copying accepted a cycle")
	}
	if len(target.Nodes) != 1 || target.Nodes[0].Next != nil {
		t.Fatal("failed decode modified the destination")
	}
}
