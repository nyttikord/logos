# Logos

Logos is a powerful Go `log/slog.Logger`.

```bash
go get -u github.com/nyttikord/logos
```

Create a new logger to `stdout`:
```go
log := logos.NewColor(io.Stdout, nil)
```

You can also write to syslog with:
```go
log, err := logos.NewSyslog("foo", syslog.LOG_USER, nil)
```
