package config

import (
	"errors"
	"math"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestCheckedConversionsAndRollback(t *testing.T) {
	type limits struct {
		U uint64  `toml:"u"`
		I int64   `toml:"i"`
		B uint8   `toml:"b"`
		F float64 `toml:"f"`
	}
	initial := limits{1, 2, 3, 4}
	for _, input := range []map[string]any{
		{"u": "18446744073709551616"}, {"u": int64(-1)}, {"i": uint64(math.MaxUint64)},
		{"i": float64(0x1p63)}, {"u": float64(0x1p64)}, {"b": 256}, {"i": 1.5},
		{"f": uint64(1<<53) + 1}, {"f": math.NaN()}, {"f": "Inf"},
	} {
		got := initial
		if err := ScanMap(input, &got); err == nil || got != initial {
			t.Fatalf("accepted or published invalid %#v: %+v (%v)", input, got, err)
		}
	}
	var got limits
	if err := ScanMap(map[string]any{"u": "18446744073709551615", "i": int64(math.MinInt64), "b": 255}, &got); err != nil || got.U != math.MaxUint64 || got.I != math.MinInt64 || got.B != 255 {
		t.Fatalf("boundaries: %+v %v", got, err)
	}
	var array struct {
		Values [2]uint16 `toml:"values"`
	}
	if err := ScanMap(map[string]any{"values": "1,65535"}, &array); err != nil || array.Values != [2]uint16{1, 65535} {
		t.Fatalf("array: %+v %v", array, err)
	}
}

func TestOwnedValuesAndSnapshots(t *testing.T) {
	type settings struct {
		Values map[string][]int `toml:"values"`
		Port   uint16           `toml:"port"`
	}
	target := settings{Values: map[string][]int{"a": {1}}, Port: 80}
	c, err := NewBuilder().WithTarget(&target).WithArgs(nil).Build()
	if err != nil {
		t.Fatal(err)
	}
	target.Values["a"][0] = 99
	first, err := c.AsStruct()
	if err != nil {
		t.Fatal(err)
	}
	if first.(*settings).Values["a"][0] != 1 {
		t.Fatal("target defaults alias internal values")
	}
	first.(*settings).Values["a"][0] = 88
	raw, _ := c.Get("values")
	raw.(map[string][]int)["a"][0] = 77
	source := map[string][]int{"a": {2}}
	if err := c.Set("values", source); err != nil {
		t.Fatal(err)
	}
	source["a"][0] = 66
	second, err := c.AsStruct()
	if err != nil {
		t.Fatal(err)
	}
	if second.(*settings).Values["a"][0] != 2 || first.(*settings).Values["a"][0] != 88 {
		t.Fatal("snapshot changed or input leaked")
	}
	clone := c.Clone()
	v, _ := clone.Get("values")
	v.(map[string][]int)["a"][0] = 44
	if err := clone.Set("values", map[string][]int{"a": {3}}); err != nil {
		t.Fatal(err)
	}
	third, _ := c.AsStruct()
	if third.(*settings).Values["a"][0] != 2 {
		t.Fatal("clone aliases original")
	}
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			for range 40 {
				s, err := c.AsStruct()
				if err != nil {
					t.Error(err)
					return
				}
				s.(*settings).Values["a"][0]++
			}
		})
	}
	wg.Wait()
}

func TestAtomicValuesAndFileRoundTrip(t *testing.T) {
	type settings struct {
		IP      net.IP        `toml:"ip"`
		Network net.IPNet     `toml:"network"`
		URL     url.URL       `toml:"url"`
		Time    *time.Time    `toml:"time"`
		Delay   time.Duration `toml:"delay"`
	}
	_, network, _ := net.ParseCIDR("10.2.0.0/16")
	endpoint, _ := url.Parse("https://user:pass@example.com/path?q=x")
	stamp := time.Date(2026, 9, 28, 12, 0, 0, 123, time.UTC)
	initial := settings{net.ParseIP("::1"), *network, *endpoint, &stamp, 1500 * time.Millisecond}
	c := New()
	if err := c.RegisterStruct("", initial); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := c.Save(path); err != nil {
		t.Fatal(err)
	}
	next := New()
	if err := next.RegisterStruct("", settings{}); err != nil {
		t.Fatal(err)
	}
	if err := next.LoadFile(path); err != nil {
		t.Fatal(err)
	}
	var got settings
	if err := next.Scan(&got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, initial) {
		t.Fatalf("atomic roundtrip: %+v != %+v", got, initial)
	}
	for _, bad := range []any{net.IP{1}, net.IPNet{IP: net.IP{1}, Mask: net.IPMask{255}}, math.NaN(), make(chan int)} {
		if err := c.Register("bad", bad); err == nil {
			t.Fatalf("accepted %T", bad)
		}
	}
	cycle := map[string]any{}
	cycle["self"] = cycle
	if err := c.Register("cycle", cycle); err == nil {
		t.Fatal("accepted cycle")
	}
	var p any
	p = &p
	if err := decodeConfig(p, new(int)); err == nil {
		t.Fatal("accepted cyclic pointer")
	}
}

func TestSourceReplacementIsTransactional(t *testing.T) {
	c := New()
	_ = c.Register("port", uint16(80))
	_ = c.Register("name", "default")
	if err := c.LoadCLI([]string{"--port=8080", "--name=old"}); err != nil {
		t.Fatal(err)
	}
	if err := c.LoadCLI([]string{"--name=new", "--port=65536"}); err == nil {
		t.Fatal("accepted overflow")
	}
	if v, _ := c.Get("name"); v != "old" {
		t.Fatal("partially published CLI")
	}
	if err := c.LoadCLI([]string{"--port=90", "--", "--name=ignored"}); err != nil {
		t.Fatal(err)
	}
	if v, _ := c.Get("name"); v != "default" {
		t.Fatal("stale CLI value")
	}
	t.Setenv("AUDIT_PORT", "100")
	if err := c.LoadEnv("AUDIT_"); err != nil {
		t.Fatal(err)
	}
	if err := os.Unsetenv("AUDIT_PORT"); err != nil {
		t.Fatal(err)
	}
	if err := c.LoadEnv("AUDIT_"); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.GetSource("port", SourceEnv); ok {
		t.Fatal("removed environment variable retained")
	}
	path := filepath.Join(t.TempDir(), "c.toml")
	_ = os.WriteFile(path, []byte("port=100\nname=\"valid\""), 0600)
	if err := c.LoadFile(path); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(path, []byte("port=65536\nname=\"invalid\""), 0600)
	if err := c.LoadFile(path); err == nil {
		t.Fatal("accepted invalid file")
	}
	if v, _ := c.GetSource("name", SourceFile); v != "valid" {
		t.Fatal("partially published file")
	}
	if err := c.SetSource(Source("invalid"), "name", "x"); err == nil {
		t.Fatal("accepted invalid source")
	}
	if err := c.Register("name.child", "x"); err == nil {
		t.Fatal("accepted overlapping path")
	}
}

func TestSaveCommentsPermissionsAndFailure(t *testing.T) {
	c := New()
	_ = c.Register("value", uint64(0))
	_ = c.Register("text", "")
	path := filepath.Join(t.TempDir(), "c.toml")
	original := "# heading\nvalue=7 # inline\ntext=\"hash # inside\"\n"
	_ = os.WriteFile(path, []byte(original), 0600)
	if err := c.LoadFile(path); err != nil {
		t.Fatal(err)
	}
	if err := c.Save(path); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	info, _ := os.Stat(path)
	if strings.Count(string(data), "# heading") != 1 || strings.Count(string(data), "# inline") != 1 || !strings.Contains(string(data), `hash # inside`) || info.Mode().Perm() != 0600 {
		t.Fatalf("save: %s mode=%v", data, info.Mode())
	}
	before := string(data)
	if err := c.Set("value", uint64(math.MaxUint64)); err != nil {
		t.Fatal(err)
	}
	if err := c.Save(path); !errors.Is(err, ErrFileFormat) {
		t.Fatalf("expected range error, got %v", err)
	}
	after, _ := os.ReadFile(path)
	if string(after) != before {
		t.Fatal("failed save changed destination")
	}
}

func FuzzConfigNumericConversions(f *testing.F) {
	for _, v := range []uint64{0, 255, 256, 1 << 63, math.MaxUint64} {
		f.Add(v)
	}
	f.Fuzz(func(t *testing.T, n uint64) {
		var u uint64
		if err := decodeConfig(n, &u); err != nil || u != n {
			t.Fatalf("uint64 loss %d %v", u, err)
		}
		var b uint8
		err := decodeConfig(n, &b)
		if (err == nil) != (n <= 255) || err == nil && uint64(b) != n {
			t.Fatalf("narrowing %d %v", b, err)
		}
		var i int64
		err = decodeConfig(n, &i)
		if (err == nil) != (n <= math.MaxInt64) || err == nil && uint64(i) != n {
			t.Fatalf("signed %d %v", i, err)
		}
	})
}
