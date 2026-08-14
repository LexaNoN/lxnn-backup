package report

import (
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	// messageLimit is a max length of a single message. Telegram allows 4096
	// characters, the rest is reserved for the markup added on rendering.
	messageLimit = 3800
	maxLineLen   = 400
	timeLayout   = "2006-01-02 15:04:05"

	okMark     = "✅"
	failMark   = "❌"
	skipMark   = "⏭"
	warnMark   = "⚠️"
	errMark    = "‼️"
	reportMark = "📦"
	updateMark = "🆕"
)

type targetInfo struct {
	Name      string
	Size      int64
	Delta     int64
	HasDelta  bool
	Ok        bool
	Delivered bool
	Duration  time.Duration
}

type jobInfo struct {
	Name          string
	Type          string
	Skipped       bool
	Err           error
	Duration      time.Duration
	Targets       []targetInfo
	Size          int64
	Delta         int64
	HasDelta      bool
	TargetsOk     int
	TargetsFailed int
}

func (j jobInfo) failed() bool {
	return !j.Skipped && (j.Err != nil || j.TargetsFailed > 0)
}

type reportData struct {
	Project         string
	Server          string
	Version         string
	StartedAt       time.Time
	Duration        time.Duration
	PrevRunAt       time.Time
	Jobs            []jobInfo
	GeneralErr      error
	StateErr        error
	TotalSize       int64
	TotalDelta      int64
	HasDelta        bool
	JobsOk          int
	JobsFailed      int
	JobsSkipped     int
	TargetsOk       int
	TargetsFailed   int
	UpdateAvailable bool
	IncludeTargets  bool
}

func (d reportData) hasProblems() bool {
	return d.GeneralErr != nil || d.JobsFailed > 0 || d.TargetsFailed > 0
}

// block is a part of the report that is never split in the middle of a line.
// Head is always visible, lines may be hidden under an expandable block quotation.
type block struct {
	head       string
	lines      []string
	expandable bool
}

// buildMessages composes the final report and splits it
// into the messages of an acceptable length
func buildMessages(d reportData) []string {
	blocks := []block{headBlock(d)}

	for _, j := range d.Jobs {
		blocks = append(blocks, jobBlock(j, d.IncludeTargets))
	}

	if b, ok := errorsBlock(d); ok {
		blocks = append(blocks, b)
	}

	return renderBlocks(blocks)
}

func headBlock(d reportData) block {
	var status string
	switch {
	case d.GeneralErr != nil || d.JobsFailed > 0:
		status = failMark + " *FAILED*"
	case len(d.Jobs) == 0:
		status = warnMark + " *NOTHING TO DO*"
	default:
		status = okMark + " *SUCCESS*"
	}

	b := block{head: fmt.Sprintf("%s *Backup report* · %s", reportMark, status)}

	if d.Project != "" {
		b.lines = append(b.lines, fmt.Sprintf("*Project:* %s", esc(d.Project)))
	}
	if d.Server != "" {
		b.lines = append(b.lines, fmt.Sprintf("*Server:* %s", esc(d.Server)))
	}
	b.lines = append(b.lines, fmt.Sprintf("*Started:* %s · *duration:* %s",
		esc(d.StartedAt.Format(timeLayout)), esc(humanDuration(d.Duration))))
	if !d.PrevRunAt.IsZero() {
		b.lines = append(b.lines, fmt.Sprintf("*Previous run:* %s", esc(d.PrevRunAt.Format(timeLayout))))
	}

	b.lines = append(b.lines, fmt.Sprintf("*Jobs:* %d · %s %d · %s %d · %s %d",
		len(d.Jobs), okMark, d.JobsOk, failMark, d.JobsFailed, skipMark, d.JobsSkipped))

	if d.TargetsOk+d.TargetsFailed > 0 {
		b.lines = append(b.lines, fmt.Sprintf("*Targets:* %d · %s %d · %s %d",
			d.TargetsOk+d.TargetsFailed, okMark, d.TargetsOk, failMark, d.TargetsFailed))
		b.lines = append(b.lines, fmt.Sprintf("*Backups size:* %s%s", esc(humanSize(d.TotalSize)), growth(d.TotalDelta, d.HasDelta)))
	}

	if d.Version != "" {
		b.lines = append(b.lines, fmt.Sprintf("*nxs\\-backup:* %s", esc(d.Version)))
	}
	if d.UpdateAvailable {
		b.lines = append(b.lines, fmt.Sprintf("%s A new version of nxs\\-backup is available", updateMark))
	}
	if d.StateErr != nil {
		b.lines = append(b.lines, fmt.Sprintf("%s Backups growth is unavailable: %s", warnMark, esc(oneLine(d.StateErr.Error()))))
	}

	return b
}

func jobBlock(j jobInfo, includeTargets bool) block {
	mark := okMark
	switch {
	case j.Skipped:
		mark = skipMark
	case j.failed():
		mark = failMark
	}

	head := fmt.Sprintf("%s *%s* · %s", mark, esc(j.Name), esc(j.Type))
	if j.Skipped {
		return block{head: head + " · skipped"}
	}

	head += fmt.Sprintf(" · %s%s · %s",
		esc(humanSize(j.Size)), growth(j.Delta, j.HasDelta), esc(humanDuration(j.Duration)))

	b := block{head: head, expandable: true}

	// errors of the job are listed in the separate errors block
	if includeTargets {
		for _, t := range j.Targets {
			b.lines = append(b.lines, targetLine(t))
		}
	}

	return b
}

func targetLine(t targetInfo) string {
	if !t.Ok {
		return fmt.Sprintf("%s %s · backup failed", failMark, code(t.Name))
	}

	l := fmt.Sprintf("%s %s · %s%s · %s",
		okMark, code(t.Name), esc(humanSize(t.Size)), growth(t.Delta, t.HasDelta), esc(humanDuration(t.Duration)))

	if !t.Delivered {
		l += " · " + warnMark + " not delivered"
	}

	return l
}

func errorsBlock(d reportData) (block, bool) {
	b := block{head: errMark + " *Errors*", expandable: true}

	// a line of the block quotation never starts with a markup character,
	// otherwise telegram may fail to detect the quotation itself
	if d.GeneralErr != nil {
		b.lines = append(b.lines, errMark+" *run*")
		b.lines = append(b.lines, errLines(d.GeneralErr)...)
	}

	for _, j := range d.Jobs {
		if j.Err == nil {
			continue
		}
		b.lines = append(b.lines, fmt.Sprintf("%s *%s*", errMark, esc(j.Name)))
		b.lines = append(b.lines, errLines(j.Err)...)
	}

	if len(b.lines) == 0 {
		return b, false
	}

	return b, true
}

// multierrHeader matches the `N errors occurred:` line the multierror starts with
var multierrHeader = regexp.MustCompile(`^\d+ errors? occurred:$`)

// errLines converts an error to the escaped report lines
func errLines(err error) []string {
	var res []string

	for _, l := range strings.Split(err.Error(), "\n") {
		l = strings.TrimSpace(l)
		if l == "" || multierrHeader.MatchString(l) {
			continue
		}
		l = strings.TrimSpace(strings.TrimPrefix(l, "* "))
		if l == "" {
			continue
		}
		res = append(res, esc(truncate(l, maxLineLen)))
	}

	return res
}

// renderBlocks joins the blocks into the messages of an acceptable length
func renderBlocks(blocks []block) []string {
	var (
		msgs []string
		cur  strings.Builder
	)

	flush := func() {
		if cur.Len() > 0 {
			msgs = append(msgs, cur.String())
			cur.Reset()
		}
	}

	for _, b := range blocks {
		for _, chunk := range b.render() {
			if cur.Len() > 0 && utf8.RuneCountInString(cur.String())+utf8.RuneCountInString(chunk)+2 > messageLimit {
				flush()
			}
			if cur.Len() > 0 {
				cur.WriteString("\n\n")
			}
			cur.WriteString(chunk)
		}
	}
	flush()

	return msgs
}

// render returns the block as one or more chunks fitting the message limit.
// The head is repeated in every chunk to keep them readable on their own.
func (b block) render() []string {
	if len(b.lines) == 0 {
		return []string{b.head}
	}

	var (
		chunks []string
		lines  []string
	)

	// `**>`, `>` and `||` markup added on rendering
	headLen := utf8.RuneCountInString(b.head) + 8

	size := headLen
	for _, l := range b.lines {
		ll := utf8.RuneCountInString(l) + 2
		if len(lines) > 0 && size+ll > messageLimit {
			chunks = append(chunks, b.renderChunk(lines))
			lines = nil
			size = headLen
		}
		lines = append(lines, l)
		size += ll
	}
	chunks = append(chunks, b.renderChunk(lines))

	return chunks
}

func (b block) renderChunk(lines []string) string {
	var sb strings.Builder

	sb.WriteString(b.head)

	if !b.expandable {
		for _, l := range lines {
			sb.WriteString("\n")
			sb.WriteString(l)
		}
		return sb.String()
	}

	// expandable block quotation: starts with `**>`, every line is prefixed
	// with `>`, the last one is marked with `||`
	for i, l := range lines {
		if i == 0 {
			sb.WriteString("\n**>")
		} else {
			sb.WriteString("\n>")
		}
		sb.WriteString(l)
	}
	sb.WriteString("||")

	return sb.String()
}

var mdEscaper = strings.NewReplacer(
	"_", "\\_",
	"*", "\\*",
	"[", "\\[",
	"]", "\\]",
	"(", "\\(",
	")", "\\)",
	"~", "\\~",
	"`", "\\`",
	">", "\\>",
	"#", "\\#",
	"+", "\\+",
	"-", "\\-",
	"=", "\\=",
	"|", "\\|",
	"{", "\\{",
	"}", "\\}",
	".", "\\.",
	"!", "\\!",
	"\\", "\\\\",
)

var codeEscaper = strings.NewReplacer(
	"\\", "\\\\",
	"`", "\\`",
)

// esc escapes the text to be used in the MarkdownV2 formatted message
func esc(s string) string {
	return mdEscaper.Replace(s)
}

// code wraps the text into the MarkdownV2 inline code entity
func code(s string) string {
	return "`" + codeEscaper.Replace(s) + "`"
}

func growth(delta int64, has bool) string {
	if !has {
		return ""
	}
	return " · " + esc(humanDelta(delta))
}

func humanSize(size int64) string {
	return humanFloatSize(float64(size))
}

func humanDelta(delta int64) string {
	switch {
	case delta > 0:
		return "+" + humanFloatSize(float64(delta))
	case delta < 0:
		return "-" + humanFloatSize(float64(-delta))
	default:
		return "±0"
	}
}

func humanFloatSize(size float64) string {
	units := []string{"B", "KiB", "MiB", "GiB", "TiB", "PiB"}

	i := 0
	for size >= 1024 && i < len(units)-1 {
		size /= 1024
		i++
	}

	if i == 0 {
		return fmt.Sprintf("%d %s", int64(size), units[i])
	}
	if size < 10 {
		return fmt.Sprintf("%.2f %s", size, units[i])
	}
	if size < 100 {
		return fmt.Sprintf("%.1f %s", size, units[i])
	}
	return fmt.Sprintf("%.0f %s", math.Round(size), units[i])
}

func humanDuration(d time.Duration) string {
	if d <= 0 {
		return "0s"
	}
	if d < time.Second {
		return d.Truncate(time.Millisecond).String()
	}
	return d.Truncate(time.Second).String()
}

func oneLine(s string) string {
	return truncate(strings.Join(strings.Fields(s), " "), maxLineLen)
}

func truncate(s string, limit int) string {
	if utf8.RuneCountInString(s) <= limit {
		return s
	}
	return string([]rune(s)[:limit]) + "…"
}
