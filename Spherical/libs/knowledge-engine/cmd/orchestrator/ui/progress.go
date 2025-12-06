// Package ui provides user interface components for the orchestrator CLI.
package ui

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/briandowns/spinner"
	"github.com/schollz/progressbar/v3"
)

// ProgressBar wraps a progressbar instance for deterministic progress display.
type ProgressBar struct {
	bar *progressbar.ProgressBar
}

// NewProgressBar creates a new progress bar with the given total and description.
func NewProgressBar(total int64, description string) *ProgressBar {
	bar := progressbar.NewOptions64(
		total,
		progressbar.OptionSetWidth(50),
		progressbar.OptionSetDescription(description),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "█",
			SaucerHead:    "█",
			SaucerPadding: "░",
			BarStart:      "│",
			BarEnd:        "│",
		}),
		progressbar.OptionSetWriter(os.Stderr),
		progressbar.OptionShowCount(),
		progressbar.OptionShowIts(),
		progressbar.OptionSetItsString("items"),
		progressbar.OptionOnCompletion(func() {
			fmt.Fprint(os.Stderr, "\n")
		}),
		progressbar.OptionSpinnerType(14),
		progressbar.OptionFullWidth(),
		progressbar.OptionSetRenderBlankState(true),
	)

	return &ProgressBar{bar: bar}
}

// Set increments the progress bar by the given amount.
func (p *ProgressBar) Set(current int64) {
	_ = p.bar.Set64(current)
}

// SetTotal updates the total value of the progress bar.
func (p *ProgressBar) SetTotal(total int64) {
	p.bar.ChangeMax64(total)
}

// Finish completes the progress bar and clears the line.
func (p *ProgressBar) Finish() {
	_ = p.bar.Finish()
}

// Spinner wraps a spinner instance for indeterminate progress display.
type Spinner struct {
	spinner *spinner.Spinner
}

// NewSpinner creates a new spinner with the given message.
func NewSpinner(message string) *Spinner {
	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = " " + message
	s.Writer = os.Stderr
	return &Spinner{spinner: s}
}

// Start starts the spinner animation.
func (s *Spinner) Start() {
	s.spinner.Start()
}

// Stop stops the spinner animation and clears the line.
func (s *Spinner) Stop() {
	s.spinner.Stop()
}

// UpdateMessage updates the spinner's message.
func (s *Spinner) UpdateMessage(message string) {
	s.spinner.Suffix = " " + message
}

// Message displays a simple message without spinner or progress bar.
func Message(format string, args ...interface{}) {
	fmt.Fprintf(os.Stdout, format, args...)
	fmt.Fprintln(os.Stdout)
}

// Error displays an error message to stderr.
func Error(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "✗ %s\n", fmt.Sprintf(format, args...))
}

// Success displays a success message.
func Success(format string, args ...interface{}) {
	fmt.Fprintf(os.Stdout, "✓ %s\n", fmt.Sprintf(format, args...))
}

// Warning displays a warning message.
func Warning(format string, args ...interface{}) {
	fmt.Fprintf(os.Stdout, "⚠ %s\n", fmt.Sprintf(format, args...))
}

// Info displays an informational message.
func Info(format string, args ...interface{}) {
	fmt.Fprintf(os.Stdout, "ℹ %s\n", fmt.Sprintf(format, args...))
}

// Newline prints a newline.
func Newline() {
	fmt.Fprintln(os.Stdout)
}

// Section displays a section header.
func Section(title string) {
	fmt.Fprintf(os.Stdout, "\n%s\n", title)
	fmt.Fprintf(os.Stdout, "%s\n\n", underline(len(title)))
}

func underline(length int) string {
	result := ""
	for i := 0; i < length; i++ {
		result += "="
	}
	return result
}

// ClearLine clears the current line (useful for progress updates).
func ClearLine(w io.Writer) {
	fmt.Fprint(w, "\r\033[K")
}

