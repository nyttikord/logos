package logos

import (
	"context"
	"io"
	"log/slog"
	"log/syslog"
	"sync"
)

// Logos represents the [slog.Handler].
//
// See [New] to create a new [Logos] with the given [Options].
type Logos struct {
	opts              Options
	goas              []groupOrAttrs
	mu                sync.Mutex
	maxFileLineLength int
	handler           Handler
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

// New creates a new [Logos] with [Handler].
//
// See [NewColor].
// See [NewSyslog].
func New(h Handler, opts *Options) *Logos {
	l := &Logos{handler: h, maxFileLineLength: 0}
	if opts != nil {
		l.opts = *opts
	}
	if l.opts.Level == nil {
		l.opts.Level = slog.LevelInfo
	}
	if l.opts.MaxFileLineLength == 0 {
		l.opts.MaxFileLineLength = 25
	}
	return l
}

// NewColor creates a new [Logos] with [ColorHandler].
//
// See [New].
// See [NewSyslog].
func NewColor(out io.Writer, opts *Options) *Logos {
	return New(ColorHandler{out: out}, opts)
}

// NewSyslog creates a new [Logos] with [SyslogHandler].
//
// See [New].
// See [NewColor].
func NewSyslog(tag string, facility syslog.Priority, opts *Options) (*Logos, error) {
	log, err := syslog.New(facility, tag)
	if err != nil {
		return nil, err
	}
	return New(SyslogHandler{log: log}, opts), nil
}

type key uint8

const (
	callerSkipKey  key = 0
	stackTraceKey  key = 1
	marshalJSONKey key = 2
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

// Enabled indicates if the given [slog.Level] is enabled.
func (l *Logos) Enabled(ctx context.Context, level slog.Level) bool {
	return level >= l.opts.Level.Level()
}
