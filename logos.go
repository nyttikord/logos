package logos

import (
	"context"
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
	// If not, they are in AnsiNotImportant (default).
	ArgsAreImportant bool
	// If TrimVersion, package versions are removed from the caller part.
	TrimVersion bool
	// If DisableColor, removes every color from logging
	DisableColor bool
	// If PrintStackTrace, error log always contains a stack trace
	PrintStackTrace bool
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
	callerSkipKey key = 0
	stackTraceKey key = 1
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
// See [FromContext] to extract the caller from a [context.Context].
func NewContext(ctx context.Context, callerSkip int, stackTrace bool) context.Context {
	ctx = context.WithValue(ctx, callerSkipKey, callerSkip)
	return context.WithValue(ctx, stackTraceKey, stackTrace)
}

// [FromContext] returns data stored in the given [context.Context].
//
// See [NewContext] to create a [context.Context].
func FromContext(ctx context.Context) (caller int, stackTrace bool, ok bool) {
	caller, ok = ctx.Value(callerSkipKey).(int)
	if !ok {
		return
	}
	stackTrace, ok = ctx.Value(stackTraceKey).(bool)
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
func (h *Logos) Enabled(ctx context.Context, level slog.Level) bool {
	return level >= h.opts.Level.Level()
}

func (h *Logos) write(color string, format string, values ...any) {
	if !h.opts.DisableColor {
		fmt.Fprint(h.out, color)
	}
	fmt.Fprintf(h.out, format, values...)
	if !h.opts.DisableColor {
		fmt.Fprint(h.out, AnsiReset)
	}
}

// Handle a [slog.Record].
func (h *Logos) Handle(ctx context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if !r.Time.IsZero() {
		h.write(AnsiNotImportant, "%s", r.Time.Format(time.DateTime))
	}
	sp := " "
	if h.opts.Align {
		var sb strings.Builder
		size := maxLength - len(r.Level.String())
		sb.Grow(size)
		for range size {
			// always returns a nil error
			sb.WriteString(" ")
		}
		sp = sb.String()
	}
	fmt.Fprint(h.out, "[")
	h.write(color(r.Level), "%s", r.Level)
	fmt.Fprint(h.out, "]", sp)
	caller, stackTrace, ok := FromContext(ctx)
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
			if !h.opts.TrimVersion || i == -1 {
				i = len(packge)
			}
			file = packge[:i] + "/" + files[len(files)-1]
		}

		fileLine := fmt.Sprintf("%s:%d", file, line)
		sp = " "
		if h.opts.Align {
			if len(fileLine) > h.opts.MaxFileLineLength {
				lineStr := strconv.Itoa(line)
				fileLine = fmt.Sprintf("...%s:%s", file[4+len(lineStr)+len(file)-h.opts.MaxFileLineLength:], lineStr)
			}
			*h.maxFileLineLength = max(len(fileLine), *h.maxFileLineLength)
			for range *h.maxFileLineLength - len(fileLine) {
				sp += " "
			}
		}
		h.write(AnsiNotImportant, "%s%s- ", fileLine, sp)
	}
	h.write(color(r.Level), "%s", r.Message)
	if !h.opts.ArgsAreImportant && !h.opts.DisableColor {
		fmt.Fprint(h.out, AnsiNotImportant)
	}
	// Handle state from WithGroup and WithAttrs.
	goas := h.goas
	if r.NumAttrs() == 0 {
		// If the record has no Attrs, remove groups at the end of the list;
		// they are empty.
		for len(goas) > 0 && goas[len(goas)-1].group != "" {
			goas = goas[:len(goas)-1]
		}
	}
	for _, goa := range goas {
		if goa.group != "" {
			fmt.Fprintf(h.out, " %s={", goa.group)
		}
		for _, a := range goa.attrs {
			h.appendAttr(h.out, a)
		}
		if goa.group != "" {
			fmt.Fprint(h.out, "}")
		}

	}
	r.Attrs(func(a slog.Attr) bool {
		h.appendAttr(h.out, a)
		fmt.Fprint(h.out, " ")
		return true
	})
	if !h.opts.DisableColor {
		fmt.Fprint(h.out, AnsiReset)
	}
	fmt.Fprint(h.out, "\n")
	if (ok && stackTrace) || (h.opts.PrintStackTrace && r.Level >= slog.LevelError) {
		debug.PrintStack()
	}
	return nil
}

func (h *Logos) appendAttr(buf io.Writer, a slog.Attr) {
	// Resolve the Attr's value before doing anything else.
	a.Value = a.Value.Resolve()
	// Ignore empty Attrs.
	if a.Equal(slog.Attr{}) {
		return
	}
	a.Key = escapeSpace(a.Key)
	fmt.Fprintf(buf, "%s=", a.Key)
	switch val := a.Value.Any().(type) {
	case fmt.Stringer:
		fmt.Fprint(buf, escapeSpace(val.String()))
		return
	case json.RawMessage:
		fmt.Fprint(buf, escapeSpace(string(val)))
		return
	case []byte:
		fmt.Fprint(buf, escapeSpace(string(val)))
		return
	case error:
		fmt.Fprint(buf, escapeSpace(val.Error()))
		return
	}
	switch a.Value.Kind() {
	case slog.KindString:
		fmt.Fprint(buf, escapeSpace(a.Value.String()))
	case slog.KindTime:
		fmt.Fprint(buf, a.Value.Time().Format(time.RFC3339))
	case slog.KindGroup:
		attrs := a.Value.Group()
		// Ignore empty groups.
		if len(attrs) == 0 {
			return
		}
		if a.Key != "" {
			fmt.Fprintf(buf, "%s=", a.Key)
		}
		fmt.Fprint(buf, "{")
		for i, ga := range attrs {
			if i > 0 {
				fmt.Fprint(buf, " ")
			}
			h.appendAttr(buf, ga)
		}
		fmt.Fprint(buf, "}")
	default:
		fmt.Fprintf(buf, "%v", a.Value.Any())
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

func (h *Logos) withGroupOrAttrs(goa groupOrAttrs) *Logos {
	h2 := *h
	h2.goas = make([]groupOrAttrs, len(h.goas)+1)
	copy(h2.goas, h.goas)
	h2.goas[len(h2.goas)-1] = goa
	return &h2
}

func (h *Logos) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	return h.withGroupOrAttrs(groupOrAttrs{group: name})
}

func (h *Logos) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}
	return h.withGroupOrAttrs(groupOrAttrs{attrs: attrs})
}
