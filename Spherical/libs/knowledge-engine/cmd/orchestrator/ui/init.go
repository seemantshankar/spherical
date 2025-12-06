package ui

import (
	"github.com/fatih/color"
)

var (
	noColorFlag bool
	verboseFlag bool
)

// InitUI initializes the UI with color and verbose settings.
func InitUI(noColor, verbose bool) {
	noColorFlag = noColor
	verboseFlag = verbose
	
	if noColor {
		color.NoColor = true
	}
}

// Close cleans up any UI resources.
func Close() {
	// Currently no cleanup needed, but can be used for future cleanup
}

// IsTerminal checks if output is going to a terminal.
func IsTerminal() bool {
	// Simple check - can be enhanced later
	return true
}

// NewUI creates a new UI instance (for compatibility, currently just a wrapper).
func NewUI(noColor, verbose bool) *UI {
	InitUI(noColor, verbose)
	return &UI{}
}

// UI is a wrapper struct for UI operations (for future state management).
type UI struct{}

