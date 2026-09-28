# Quick start

Use Go 1.27.1 and TOML tags. Register defaults before loading sources:

```go
cfg := config.New()
if err := cfg.Register("server.port", uint16(8080)); err != nil { return err }
if err := cfg.LoadFile("config.toml"); err != nil { return err }
if err := cfg.LoadEnv("APP_"); err != nil { return err }
if err := cfg.LoadCLI(os.Args[1:]); err != nil { return err }
port, err := config.GetTyped[uint16](cfg, "server.port")
```

`APP_SERVER_PORT=9000` and `--server.port=9001` address the same registered path.
CLI has highest default priority. TOML values can be configured in a table:

```toml
[server]
port = 8080
```

For a typed application, use `NewBuilder().WithTarget(&settings)` as shown in the
[README](../README.md). Build fills the supplied target once. Use `AsStruct` to
obtain new snapshots after updates; retaining an old snapshot is safe.

A missing file returns `ErrConfigNotFound` along with a usable Config when all
other initialization succeeds. Check that category explicitly if a file is
optional. Invalid environment or CLI values remain fatal even when the file is
missing. `MustBuild` and `MustQuick` ignore only a missing file and panic on other
initialization errors. All source loads replace the previous values of that source.
