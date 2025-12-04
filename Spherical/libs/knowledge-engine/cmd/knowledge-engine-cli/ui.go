// Package main provides UI utilities for the Knowledge Engine CLI.
package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
)

// UI provides user-friendly output utilities.
type UI struct {
	progress *mpb.Progress
	noColor  bool
	jsonMode bool
}

// NewUI creates a new UI instance.
func NewUI(jsonMode, noColor bool) *UI {
	var progress *mpb.Progress
	if !jsonMode {
		progress = mpb.New(mpb.WithWidth(64))
	}
	return &UI{
		progress: progress,
		noColor:  noColor,
		jsonMode: jsonMode,
	}
}

// Close closes the UI and cleans up resources.
func (ui *UI) Close() {
	if ui.progress != nil {
		// Only wait if we're in a terminal (not piped)
		// When piped, progress bars can't render and Wait() may hang
		if IsTerminal() {
			ui.progress.Wait()
		} else {
			// For piped output, shutdown without waiting to avoid hanging
			// The progress bars won't render anyway when piped
			ui.progress.Shutdown()
		}
	}
}

// Success prints a success message.
func (ui *UI) Success(format string, args ...interface{}) {
	if ui.jsonMode {
		return
	}
	if ui.noColor {
		fmt.Printf("✓ %s\n", fmt.Sprintf(format, args...))
	} else {
		color.New(color.FgGreen).Printf("✓ %s\n", fmt.Sprintf(format, args...))
	}
}

// Error prints an error message.
func (ui *UI) Error(format string, args ...interface{}) {
	if ui.jsonMode {
		return
	}
	if ui.noColor {
		fmt.Fprintf(os.Stderr, "✗ %s\n", fmt.Sprintf(format, args...))
	} else {
		color.New(color.FgRed).Printf("✗ %s\n", fmt.Sprintf(format, args...))
	}
}

// Warning prints a warning message.
func (ui *UI) Warning(format string, args ...interface{}) {
	if ui.jsonMode {
		return
	}
	if ui.noColor {
		fmt.Printf("⚠ %s\n", fmt.Sprintf(format, args...))
	} else {
		color.New(color.FgYellow).Printf("⚠ %s\n", fmt.Sprintf(format, args...))
	}
}

// Info prints an info message.
func (ui *UI) Info(format string, args ...interface{}) {
	if ui.jsonMode {
		return
	}
	if ui.noColor {
		fmt.Printf("ℹ %s\n", fmt.Sprintf(format, args...))
	} else {
		color.New(color.FgCyan).Printf("ℹ %s\n", fmt.Sprintf(format, args...))
	}
}

// Step prints a step message.
func (ui *UI) Step(format string, args ...interface{}) {
	if ui.jsonMode {
		return
	}
	if ui.noColor {
		fmt.Printf("→ %s\n", fmt.Sprintf(format, args...))
	} else {
		color.New(color.FgBlue).Printf("→ %s\n", fmt.Sprintf(format, args...))
	}
}

// ProgressBar creates a new progress bar.
func (ui *UI) ProgressBar(name string, total int64) *mpb.Bar {
	if ui.progress == nil || ui.jsonMode {
		return nil
	}

	bar := ui.progress.AddBar(total,
		mpb.PrependDecorators(
			decor.Name(name, decor.WC{W: len(name) + 1, C: decor.DSyncSpaceR}),
			decor.CountersNoUnit("%d / %d", decor.WCSyncWidth),
		),
		mpb.AppendDecorators(
			decor.Percentage(decor.WC{W: 5}),
			decor.Elapsed(decor.ET_STYLE_GO, decor.WC{W: 12}),
			decor.OnComplete(
				decor.AverageETA(decor.ET_STYLE_GO, decor.WC{W: 12}),
				" done",
			),
		),
	)

	return bar
}

// Spinner creates a spinner for indeterminate progress.
func (ui *UI) Spinner(name string) *mpb.Bar {
	if ui.progress == nil || ui.jsonMode {
		return nil
	}

	// Create a spinner using AddBar with a large total to simulate indeterminate progress
	spinner := ui.progress.AddBar(100,
		mpb.BarFillerOnComplete("✓"),
		mpb.PrependDecorators(
			decor.Name(name, decor.WC{W: len(name) + 1, C: decor.DSyncSpaceR}),
			decor.Spinner([]string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}, decor.WC{W: 1}),
		),
		mpb.AppendDecorators(
			decor.Elapsed(decor.ET_STYLE_GO, decor.WC{W: 12}),
		),
	)

	return spinner
}

// Table prints a formatted table.
func (ui *UI) Table(headers []string, rows [][]string) {
	if ui.jsonMode {
		return
	}

	if len(headers) == 0 {
		return
	}

	// Calculate column widths
	widths := make([]int, len(headers))
	for i, header := range headers {
		widths[i] = len(header)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i < len(widths) && len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}

	// Print header
	if !ui.noColor {
		color.New(color.FgCyan, color.Bold).Print("┌")
		for i, width := range widths {
			fmt.Print(strings.Repeat("─", width+2))
			if i < len(widths)-1 {
				color.New(color.FgCyan, color.Bold).Print("┬")
			}
		}
		color.New(color.FgCyan, color.Bold).Print("┐\n")
	} else {
		fmt.Print("+")
		for i, width := range widths {
			fmt.Print(strings.Repeat("-", width+2))
			if i < len(widths)-1 {
				fmt.Print("+")
			}
		}
		fmt.Print("+\n")
	}

	// Print header row
	if !ui.noColor {
		color.New(color.FgCyan, color.Bold).Print("│")
	} else {
		fmt.Print("|")
	}
	for i, header := range headers {
		if i < len(widths) {
			fmt.Printf(" %-*s ", widths[i], header)
			if i < len(headers)-1 {
				if !ui.noColor {
					color.New(color.FgCyan, color.Bold).Print("│")
				} else {
					fmt.Print("|")
				}
			}
		}
	}
	if !ui.noColor {
		color.New(color.FgCyan, color.Bold).Print("│\n")
	} else {
		fmt.Print("|\n")
	}

	// Print separator
	if !ui.noColor {
		color.New(color.FgCyan, color.Bold).Print("├")
		for i, width := range widths {
			fmt.Print(strings.Repeat("─", width+2))
			if i < len(widths)-1 {
				color.New(color.FgCyan, color.Bold).Print("┼")
			}
		}
		color.New(color.FgCyan, color.Bold).Print("┤\n")
	} else {
		fmt.Print("+")
		for i, width := range widths {
			fmt.Print(strings.Repeat("-", width+2))
			if i < len(widths)-1 {
				fmt.Print("+")
			}
		}
		fmt.Print("+\n")
	}

	// Print rows
	for _, row := range rows {
		if !ui.noColor {
			fmt.Print("│")
		} else {
			fmt.Print("|")
		}
		for i, cell := range row {
			if i < len(widths) {
				fmt.Printf(" %-*s ", widths[i], cell)
				if i < len(row)-1 {
					if !ui.noColor {
						fmt.Print("│")
					} else {
						fmt.Print("|")
					}
				}
			}
		}
		if !ui.noColor {
			fmt.Print("│\n")
		} else {
			fmt.Print("|\n")
		}
	}

	// Print footer
	if !ui.noColor {
		color.New(color.FgCyan, color.Bold).Print("└")
		for i, width := range widths {
			fmt.Print(strings.Repeat("─", width+2))
			if i < len(widths)-1 {
				color.New(color.FgCyan, color.Bold).Print("┴")
			}
		}
		color.New(color.FgCyan, color.Bold).Print("┘\n")
	} else {
		fmt.Print("+")
		for i, width := range widths {
			fmt.Print(strings.Repeat("-", width+2))
			if i < len(widths)-1 {
				fmt.Print("+")
			}
		}
		fmt.Print("+\n")
	}
}

// Section prints a section header.
func (ui *UI) Section(title string) {
	if ui.jsonMode {
		return
	}
	fmt.Println()
	if ui.noColor {
		fmt.Printf("━━━ %s ━━━\n", strings.ToUpper(title))
	} else {
		color.New(color.FgMagenta, color.Bold).Printf("━━━ %s ━━━\n", strings.ToUpper(title))
	}
	fmt.Println()
}

// KeyValue prints a key-value pair.
func (ui *UI) KeyValue(key string, value interface{}) {
	if ui.jsonMode {
		return
	}
	if ui.noColor {
		fmt.Printf("  %s: %v\n", key, value)
	} else {
		color.New(color.FgYellow).Printf("  %s: ", key)
		fmt.Printf("%v\n", value)
	}
}

// Newline prints a newline.
func (ui *UI) Newline() {
	if !ui.jsonMode {
		fmt.Println()
	}
}

// FormatDuration formats a duration in a human-readable way.
func FormatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	if d < time.Hour {
		return fmt.Sprintf("%.1fm", d.Minutes())
	}
	return fmt.Sprintf("%.1fh", d.Hours())
}

// FormatBytes formats bytes in a human-readable way.
func FormatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// IsTerminal checks if stdout is a terminal.
func IsTerminal() bool {
	fileInfo, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fileInfo.Mode() & os.ModeCharDevice) != 0
}

