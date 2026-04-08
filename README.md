# Logos

Logos is a colorful Go `log/slog.Logger`.

```bash
go get -u github.com/nyttikord/logos
```

Create a new logger to `stdout`:
```go
log := logos.New(io.Stdout, nil)
```
