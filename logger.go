package logos

import (
	"context"
	"encoding"
	"encoding/json"
	"fmt"
	"log/slog"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"time"
)

// Handle a [slog.Record].
func (l *Logos) Handle(ctx context.Context, r slog.Record) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	var t string
	if !r.Time.IsZero() {
		t = r.Time.Format(time.DateTime)
	}
	sp := " "
	if l.opts.Align {
		var sb strings.Builder
		size := maxLength - len(r.Level.String())
		sb.Grow(size)
		for range size {
			// always returns a nil error
			sb.WriteString(" ")
		}
		sp = sb.String()
	}
	caller, stackTrace, marshalJSON, ok := FromContext(ctx)
	defer func(before bool) {
		l.opts.MarshalJSON = before
	}(l.opts.MarshalJSON)
	l.opts.MarshalJSON = marshalJSON
	var fileLine string
	if r.PC != 0 {
		var file string
		var line int
		if ok {
			_, file, line, ok = runtime.Caller(caller + 3)
		} else {
			_, file, line, ok = runtime.Caller(3)
		}
		files := strings.Split(file, "/")
		if len(files) == 1 {
			file = files[len(files)-1]
		} else {
			// remove package version from log
			packge := files[len(files)-2]
			i := strings.Index(packge, "@")
			if !l.opts.TrimVersion || i == -1 {
				i = len(packge)
			}
			file = packge[:i] + "/" + files[len(files)-1]
		}

		lineStr := strconv.Itoa(line)
		var sb strings.Builder
		sb.Grow(len(sp) + len(file) + len(lineStr) + 2)
		sb.WriteString(sp)
		ln := len(file) + len(lineStr) + 1
		if ln > l.opts.MaxFileLineLength {
			sb.WriteString("...")
			sb.WriteString(file[4+len(lineStr)+len(file)-l.opts.MaxFileLineLength:])
		} else {
			sb.WriteString(file)
		}
		ln = min(ln, l.opts.MaxFileLineLength)
		sb.WriteRune(':')
		sb.WriteString(lineStr)
		sb.WriteRune(' ')
		if l.opts.Align {
			*l.maxFileLineLength = max(ln, *l.maxFileLineLength)
			add := *l.maxFileLineLength - ln
			sb.Grow(add)
			for range add {
				sb.WriteRune(' ')
			}
		}
		fileLine = sb.String()
	}
	// Handle state from WithGroup and WithAttrs.
	goas := l.goas
	if r.NumAttrs() == 0 {
		// If the record has no Attrs, remove groups at the end of the list;
		// they are empty.
		for len(goas) > 0 && goas[len(goas)-1].group != "" {
			goas = goas[:len(goas)-1]
		}
	}
	var arg strings.Builder
	arg.Grow(len(goas) * 7)
	for _, goa := range goas {
		if goa.group != "" {
			arg.WriteString(goa.group)
			arg.WriteString("={")
		}
		for _, a := range goa.attrs {
			arg.WriteString(l.appendAttr(a))
		}
		if goa.group != "" {
			arg.WriteRune('}')
		}
	}
	arg.Grow(r.NumAttrs() * 4)
	r.Attrs(func(a slog.Attr) bool {
		arg.WriteString(l.appendAttr(a))
		return true
	})
	var stack []byte = nil
	if (ok && stackTrace) || (l.opts.PrintStackTrace && r.Level >= slog.LevelError) {
		stack = debug.Stack()
	}
	l.handler.Write(l.opts, r.Level, t, fileLine, r.Message, arg.String(), stack)
	return nil
}

func (l *Logos) appendAttr(a slog.Attr) string {
	// Resolve the Attr's value before doing anything else.
	a.Value = a.Value.Resolve()
	// Ignore empty Attrs.
	if a.Equal(slog.Attr{}) {
		return ""
	}
	var sb strings.Builder
	sb.WriteRune(' ')
	a.Key = escapeSpace(a.Key)
	sb.WriteString(a.Key)
	sb.WriteRune('=')
	switch val := a.Value.Any().(type) {
	case fmt.Stringer:
		sb.WriteString(escapeSpace(val.String()))
		return sb.String()
	case encoding.TextMarshaler:
		t, err := val.MarshalText()
		if err == nil {
			sb.WriteString(escapeSpace(string(t)))
			return sb.String()
		}
	case json.RawMessage:
		sb.WriteString(escapeSpace(string(val)))
		return sb.String()
	case json.Marshaler:
		if l.opts.MarshalJSON {
			b, err := val.MarshalJSON()
			if err == nil {
				sb.WriteString(escapeSpace(string(b)))
				return sb.String()
			}
		}
	case []byte:
		sb.WriteString(escapeSpace(string(val)))
		return sb.String()
	case error:
		sb.WriteString(escapeSpace(val.Error()))
		return sb.String()
	}
	switch a.Value.Kind() {
	case slog.KindString:
		sb.WriteString(escapeSpace(a.Value.String()))
		return sb.String()
	case slog.KindTime:
		sb.WriteString(a.Value.Time().Format(time.RFC3339))
		return sb.String()
	case slog.KindGroup:
		attrs := a.Value.Group()
		// Ignore empty groups.
		if len(attrs) == 0 {
			return ""
		}
		if a.Key != "" {
			sb.WriteString(a.Key)
			sb.WriteRune('=')
		}
		sb.WriteRune('{')
		sb.Grow(len(attrs) * 4)
		for _, ga := range attrs {
			sb.WriteString(l.appendAttr(ga))
		}
		sb.WriteRune('}')
		return sb.String()
	default:
		fmt.Fprintf(&sb, "%v", a.Value.Any())
		return sb.String()
	}
}

func escapeSpace(s string) string {
	if strings.Contains(s, " ") {
		s = fmt.Sprintf("%q", s)
	}
	s = strings.ReplaceAll(s, "\n", `\n`)
	return s
}
