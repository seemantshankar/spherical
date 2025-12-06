package ui

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"
)

// Table displays data in a formatted table.
func Table(headers []string, rows [][]string) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	
	// Print headers
	fmt.Fprintln(w, strings.Join(headers, "\t"))
	
	// Print separator
	separator := make([]string, len(headers))
	for i := range separator {
		separator[i] = strings.Repeat("-", len(headers[i]))
	}
	fmt.Fprintln(w, strings.Join(separator, "\t"))
	
	// Print rows
	for _, row := range rows {
		fmt.Fprintln(w, strings.Join(row, "\t"))
	}
	
	_ = w.Flush()
}

// Box displays text in a box with borders.
func Box(title string, content string) {
	lines := strings.Split(content, "\n")
	maxWidth := len(title)
	for _, line := range lines {
		// Remove any existing box characters from width calculation
		cleanLine := strings.TrimSpace(strings.Trim(line, "│┃║"))
		if len(cleanLine) > maxWidth {
			maxWidth = len(cleanLine)
		}
	}
	
	// Ensure minimum width
	if maxWidth < 40 {
		maxWidth = 40
	}
	
	// Use simpler, more visible box characters
	topLeft := "┌"
	topRight := "┐"
	bottomLeft := "└"
	bottomRight := "┘"
	horizontal := "─"
	vertical := "│"
	
	// Top border
	fmt.Printf("%s%s%s\n", topLeft, strings.Repeat(horizontal, maxWidth+2), topRight)
	
	// Title
	if title != "" {
		fmt.Printf("%s %-*s %s\n", vertical, maxWidth, title, vertical)
		fmt.Printf("%s%s%s\n", "├", strings.Repeat(horizontal, maxWidth+2), "┤")
	}
	
	// Content
	for _, line := range lines {
		// Remove existing box characters if present
		cleanLine := strings.TrimSpace(strings.Trim(line, "│┃║"))
		if cleanLine == "" {
			fmt.Printf("%s %-*s %s\n", vertical, maxWidth, "", vertical)
		} else {
			fmt.Printf("%s %-*s %s\n", vertical, maxWidth, cleanLine, vertical)
		}
	}
	
	// Bottom border
	fmt.Printf("%s%s%s\n", bottomLeft, strings.Repeat(horizontal, maxWidth+2), bottomRight)
}

// WarningBox displays a warning message in a box.
func WarningBox(title, message string) {
	fmt.Fprintf(os.Stdout, "\n")
	Box("⚠️  "+title, message)
	fmt.Fprintf(os.Stdout, "\n")
}

// ErrorBox displays an error message in a box.
func ErrorBox(title, message string) {
	fmt.Fprintf(os.Stderr, "\n")
	Box("✗ "+title, message)
	fmt.Fprintf(os.Stderr, "\n")
}

// SuccessBox displays a success message in a box.
func SuccessBox(title, message string) {
	fmt.Fprintf(os.Stdout, "\n")
	Box("✓ "+title, message)
	fmt.Fprintf(os.Stdout, "\n")
}

// FormatList formats a list of items as bullets.
func FormatList(items []string) string {
	var sb strings.Builder
	for _, item := range items {
		sb.WriteString(fmt.Sprintf("  • %s\n", item))
	}
	return sb.String()
}

// FormatDuration formats a duration in a human-readable way.
func FormatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	
	hours := d / time.Hour
	d -= hours * time.Hour
	minutes := d / time.Minute
	d -= minutes * time.Minute
	seconds := d / time.Second
	
	if hours > 0 {
		return fmt.Sprintf("%dh %dm %ds", hours, minutes, seconds)
	}
	if minutes > 0 {
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	}
	return fmt.Sprintf("%ds", seconds)
}

// KeyValue displays a key-value pair in a formatted way.
func KeyValue(key, value string) {
	fmt.Fprintf(os.Stdout, "  %s: %s\n", key, value)
}

// Step displays a step indicator message.
func Step(format string, args ...interface{}) {
	fmt.Fprintf(os.Stdout, "→ %s\n", fmt.Sprintf(format, args...))
}

