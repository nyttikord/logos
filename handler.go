package logos

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"log/syslog"
	"os"
	"strings"
)

// Handler writes the content of [Logos].
type Handler interface {
	Write(ctx context.Context, opts Options, level slog.Level, time, file, content, arg string, stack []byte)
}

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

// ColorHandler is the previous default [Handler] of [Logos].
// It produces a colorful output to an [io.Writer].
type ColorHandler struct {
	out io.Writer
}

func (c ColorHandler) Write(
	ctx context.Context, opts Options, level slog.Level, time, file, content, arg string, stack []byte,
) {
	if time != "" {
		c.writeColor(opts, AnsiNotImportant, time)
	}
	var colorLevel string
	switch {
	case level >= slog.LevelError:
		colorLevel = AnsiRed
	case level >= slog.LevelWarn:
		colorLevel = AnsiYellow
	case level >= slog.LevelInfo:
		colorLevel = AnsiGreen
	default:
		colorLevel = AnsiReset
	}
	fmt.Fprint(c.out, "[")
	c.writeColor(opts, colorLevel, level.String())
	fmt.Fprint(c.out, "]")
	c.writeColor(opts, AnsiNotImportant, file+"- ")
	c.writeColor(opts, colorLevel, content)
	if !opts.ArgsAreImportant && !opts.DisableColor {
		fmt.Fprint(c.out, AnsiNotImportant)
	}
	fmt.Fprint(c.out, arg)
	if !opts.DisableColor {
		fmt.Fprint(c.out, AnsiReset)
	}
	fmt.Fprint(c.out, "\n")
	if stack != nil {
		os.Stderr.Write(stack)
	}
}

func (c ColorHandler) writeColor(opts Options, color, msg string) {
	if opts.DisableColor {
		fmt.Fprint(c.out, msg)
		return
	}
	fmt.Fprintf(c.out, "%s%s%s", color, msg, AnsiReset)
}

// groupOrAttrs holds either a group name or a list of slog.Attrs.
type groupOrAttrs struct {
	group string      // group name if non-empty
	attrs []slog.Attr // attrs if non-empty
}

func (l *Logos) withGroupOrAttrs(goa groupOrAttrs) *Logos {
	l2 := Logos{opts: l.opts, handler: l.handler}
	l2.goas = make([]groupOrAttrs, len(l.goas)+1)
	copy(l2.goas, l.goas)
	l2.goas[len(l2.goas)-1] = goa
	return &l2
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

type SyslogHandler struct {
	log *syslog.Writer
	tag string
}

func (s SyslogHandler) Write(
	ctx context.Context, opts Options, level slog.Level, time, file, content, arg string, stack []byte,
) {
	log := s.log
	if f, ok := FromSyslogContext(ctx); ok {
		l, err := syslog.New(f, s.tag)
		if err == nil {
			log = l
			defer log.Close()
		} else {
			_ = log.Err("cannot open new syslog connection, using default one")
		}
	}
	var fn func(string) error
	switch {
	case level < slog.LevelInfo:
		fn = log.Debug
	case level < slog.LevelWarn:
		fn = log.Info
	case level < slog.LevelError:
		fn = log.Warning
	default:
		fn = log.Err
	}
	var sb strings.Builder
	sb.Grow(len(file) + len(content) + len(arg) + 2)
	sb.WriteString(file)
	sb.WriteString("- ")
	sb.WriteString(content)
	sb.WriteString(arg)
	if stack != nil {
		sb.WriteRune('\n')
		sb.Write(stack)
	}
	_ = fn(sb.String())
}
