package logos

import (
	"context"
	"encoding"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	AnsiReset       = "\033[0m"
	AnsiRed         = "\033[38;5;9m"
	AnsiGrey        = "\033[38;5;244m"
	AnsiGreen       = "\033[38;5;2m"
	AnsiYellow      = "\033[38;5;11m"
	AnsiBlue        = "\033[38;5;6m"
	AnsiMagenta     = "\033[38;5;13m"
	AnsiCyan        = "\033[38;5;14m"
	AnsiWhite       = "\033[37m"
	AnsiBlueBold    = "\033[34;1m"
	AnsiMagentaBold = "\033[35;1m"
	AnsiRedBold     = "\033[31;1m"
	AnsiYellowBold  = "\033[33;1m"

	AnsiNotImportant = AnsiGrey
)

// Logos represents the [slog.Handler].
//
// See [New] to create a new [Logos] with the given [Options].
type Logos struct {
	opts              Options
	goas              []groupOrAttrs
	mu                *sync.Mutex
	out               io.Writer
	maxFileLineLength *int
}

// Options of [Logos].
type Options struct {
	// Level reports the minimum level to log.
	// Levels with lower levels are discarded.
	// If nil, the Handler uses [slog.LevelInfo].
	Level slog.Leveler

	// MaxFileLineLength is the maximum length of the caller part.
	// Default value is 25.
	MaxFileLineLength int
	// If Align, everything logged will be aligned dynamically.
	Align bool
	// If ArgsAreImportant, args are in default terminal color.
	// If not, they are in [AnsiNotImportant] (default).
	ArgsAreImportant bool
	// If TrimVersion, package versions are removed from the caller part.
	TrimVersion bool
	// If DisableColor, removes every color from logging
	DisableColor bool
	// If PrintStackTrace, error log always contains a stack trace
	PrintStackTrace bool
	// If MarshalJSON, types implementing [json.Marshaler] will be marshaled into JSON.
	MarshalJSON bool
}

// New creates a new [Logos].
func New(out io.Writer, opts *Options) *Logos {
	h := &Logos{out: out, mu: new(sync.Mutex), maxFileLineLength: new(int)}
	if opts != nil {
		h.opts = *opts
	}
	if h.opts.Level == nil {
		h.opts.Level = slog.LevelInfo
	}
	if h.opts.MaxFileLineLength == 0 {
		h.opts.MaxFileLineLength = 25
	}
	return h
}

type key int

const (
	callerSkipKey  key = 0
	stackTraceKey  key = 1
	marshalJSONKey key = 2
)

var maxLength = max(
	len(slog.LevelDebug.String()),
	len(slog.LevelInfo.String()),
	len(slog.LevelWarn.String()),
	len(slog.LevelError.String()),
)

// NewContext returns a new [context.Context] with the callerSkip given.
//
// callerSkip is the number of runtime calls to log before this one.
// 0 is for the current.
// 1 is for the precedent call.
// n is for the n times precedent call.
// The calls to the log is already skipped.
//
// stackTrace and marshalJSON overrides [Options.MarshalJSON] and [Options.PrintStackTrace] for the current call.
//
// See [FromContext] to extract the caller from a [context.Context].
func NewContext(ctx context.Context, callerSkip int, stackTrace, marshalJSON bool) context.Context {
	ctx = context.WithValue(ctx, callerSkipKey, callerSkip)
	ctx = context.WithValue(ctx, stackTraceKey, stackTrace)
	ctx = context.WithValue(ctx, marshalJSONKey, marshalJSON)
	return ctx
}

// [FromContext] returns data stored in the given [context.Context].
//
// See [NewContext] to create a [context.Context].
func FromContext(ctx context.Context) (caller int, stackTrace, marshalJSON, ok bool) {
	caller, ok = ctx.Value(callerSkipKey).(int)
	if !ok {
		return
	}
	stackTrace, ok = ctx.Value(stackTraceKey).(bool)
	if !ok {
		return
	}
	marshalJSON, ok = ctx.Value(marshalJSONKey).(bool)
	return
}

func color(level slog.Level) string {
	if level >= slog.LevelError {
		return AnsiRed
	} else if level >= slog.LevelWarn {
		return AnsiYellow
	} else if level >= slog.LevelInfo {
		return AnsiGreen
	} else {
		return AnsiReset
	}
}

// Enabled indicates if the given [slog.Level] is enabled.
func (l *Logos) Enabled(ctx context.Context, level slog.Level) bool {
	return level >= l.opts.Level.Level()
}

func (l *Logos) write(color string, format string, values ...any) {
	if !l.opts.DisableColor {
		fmt.Fprint(l.out, color)
	}
	fmt.Fprintf(l.out, format, values...)
	if !l.opts.DisableColor {
		fmt.Fprint(l.out, AnsiReset)
	}
}

// Handle a [slog.Record].
func (l *Logos) Handle(ctx context.Context, r slog.Record) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !r.Time.IsZero() {
		l.write(AnsiNotImportant, "%s ", r.Time.Format(time.DateTime))
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
	fmt.Fprint(l.out, "[")
	l.write(color(r.Level), "%s", r.Level)
	fmt.Fprint(l.out, "]", sp)
	caller, stackTrace, marshalJSON, ok := FromContext(ctx)
	defer func(before bool) {
		l.opts.MarshalJSON = before
	}(l.opts.MarshalJSON)
	l.opts.MarshalJSON = marshalJSON
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

		fileLine := fmt.Sprintf("%s:%d", file, line)
		sp = " "
		if l.opts.Align {
			if len(fileLine) > l.opts.MaxFileLineLength {
				lineStr := strconv.Itoa(line)
				fileLine = fmt.Sprintf("...%s:%s", file[4+len(lineStr)+len(file)-l.opts.MaxFileLineLength:], lineStr)
			}
			*l.maxFileLineLength = max(len(fileLine), *l.maxFileLineLength)
			for range *l.maxFileLineLength - len(fileLine) {
				sp += " "
			}
		}
		l.write(AnsiNotImportant, "%s%s- ", fileLine, sp)
	}
	l.write(color(r.Level), "%s", r.Message)
	if !l.opts.ArgsAreImportant && !l.opts.DisableColor {
		fmt.Fprint(l.out, AnsiNotImportant)
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
	for _, goa := range goas {
		if goa.group != "" {
			fmt.Fprintf(l.out, " %s={", goa.group)
		}
		for _, a := range goa.attrs {
			l.appendAttr(l.out, a)
		}
		if goa.group != "" {
			fmt.Fprint(l.out, "}")
		}

	}
	r.Attrs(func(a slog.Attr) bool {
		l.appendAttr(l.out, a)
		return true
	})
	if !l.opts.DisableColor {
		fmt.Fprint(l.out, " ")
		fmt.Fprint(l.out, AnsiReset)
	}
	fmt.Fprint(l.out, "\n")
	if (ok && stackTrace) || (l.opts.PrintStackTrace && r.Level >= slog.LevelError) {
		debug.PrintStack()
	}
	return nil
}

func (l *Logos) appendAttr(w io.Writer, a slog.Attr) {
	// Resolve the Attr's value before doing anything else.
	a.Value = a.Value.Resolve()
	// Ignore empty Attrs.
	if a.Equal(slog.Attr{}) {
		return
	}
	fmt.Fprint(w, " ")
	a.Key = escapeSpace(a.Key)
	fmt.Fprintf(w, "%s=", a.Key)
	switch val := a.Value.Any().(type) {
	case fmt.Stringer:
		fmt.Fprint(w, escapeSpace(val.String()))
		return
	case encoding.TextMarshaler:
		t, err := val.MarshalText()
		if err == nil {
			fmt.Fprint(w, escapeSpace(string(t)))
			return
		}
	case json.RawMessage:
		fmt.Fprint(w, escapeSpace(string(val)))
		return
	case json.Marshaler:
		if l.opts.MarshalJSON {
			b, err := val.MarshalJSON()
			if err == nil {
				fmt.Fprint(w, escapeSpace(string(b)))
				return
			}
		}
	case []byte:
		fmt.Fprint(w, escapeSpace(string(val)))
		return
	case error:
		fmt.Fprint(w, escapeSpace(val.Error()))
		return
	}
	switch a.Value.Kind() {
	case slog.KindString:
		fmt.Fprint(w, escapeSpace(a.Value.String()))
	case slog.KindTime:
		fmt.Fprint(w, a.Value.Time().Format(time.RFC3339))
	case slog.KindGroup:
		attrs := a.Value.Group()
		// Ignore empty groups.
		if len(attrs) == 0 {
			return
		}
		if a.Key != "" {
			fmt.Fprintf(w, "%s=", a.Key)
		}
		fmt.Fprint(w, "{")
		for _, ga := range attrs {
			l.appendAttr(w, ga)
		}
		fmt.Fprint(w, "}")
	default:
		fmt.Fprintf(w, "%v", a.Value.Any())
	}
}

func escapeSpace(s string) string {
	if strings.Contains(s, " ") {
		s = fmt.Sprintf("%q", s)
	}
	s = strings.ReplaceAll(s, "\n", `\n`)
	return s
}

// groupOrAttrs holds either a group name or a list of slog.Attrs.
type groupOrAttrs struct {
	group string      // group name if non-empty
	attrs []slog.Attr // attrs if non-empty
}

func (l *Logos) withGroupOrAttrs(goa groupOrAttrs) *Logos {
	h2 := *l
	h2.goas = make([]groupOrAttrs, len(l.goas)+1)
	copy(h2.goas, l.goas)
	h2.goas[len(h2.goas)-1] = goa
	return &h2
}

func (l *Logos) WithGroup(name string) slog.Handler {
	if name == "" {
		return l
	}
	return l.withGroupOrAttrs(groupOrAttrs{group: name})
}

func (l *Logos) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return l
	}
	return l.withGroupOrAttrs(groupOrAttrs{attrs: attrs})
}
