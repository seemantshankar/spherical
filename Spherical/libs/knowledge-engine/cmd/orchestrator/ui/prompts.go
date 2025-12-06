package ui

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// Prompt asks the user for input with a prompt message.
func Prompt(message string) (string, error) {
	fmt.Fprintf(os.Stdout, "%s: ", message)
	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(input), nil
}

// PromptWithDefault asks the user for input with a default value.
func PromptWithDefault(message, defaultValue string) (string, error) {
	prompt := fmt.Sprintf("%s [%s]", message, defaultValue)
	fmt.Fprintf(os.Stdout, "%s: ", prompt)
	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return defaultValue, nil
	}
	return trimmed, nil
}

// Confirm asks the user for a yes/no confirmation.
func Confirm(message string, defaultValue bool) (bool, error) {
	defaultStr := "y/N"
	if defaultValue {
		defaultStr = "Y/n"
	}
	
	prompt := fmt.Sprintf("%s [%s]", message, defaultStr)
	fmt.Fprintf(os.Stdout, "%s: ", prompt)
	
	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return false, err
	}
	
	trimmed := strings.ToLower(strings.TrimSpace(input))
	
	if trimmed == "" {
		return defaultValue, nil
	}
	
	return trimmed == "y" || trimmed == "yes", nil
}

// PromptInt asks the user for an integer input.
func PromptInt(message string) (int, error) {
	input, err := Prompt(message)
	if err != nil {
		return 0, err
	}
	
	var value int
	_, err = fmt.Sscanf(input, "%d", &value)
	if err != nil {
		return 0, fmt.Errorf("invalid number: %w", err)
	}
	
	return value, nil
}

// PromptChoice asks the user to select from a list of choices.
func PromptChoice(message string, choices []string) (int, error) {
	fmt.Fprintf(os.Stdout, "%s\n", message)
	for i, choice := range choices {
		fmt.Fprintf(os.Stdout, "  %d. %s\n", i+1, choice)
	}
	
	input, err := Prompt("Enter your choice")
	if err != nil {
		return 0, err
	}
	
	var choice int
	_, err = fmt.Sscanf(input, "%d", &choice)
	if err != nil {
		return 0, fmt.Errorf("invalid choice: %w", err)
	}
	
	if choice < 1 || choice > len(choices) {
		return 0, fmt.Errorf("choice must be between 1 and %d", len(choices))
	}
	
	return choice - 1, nil // Return 0-indexed
}

// PromptRequired confirms that input is not empty.
func PromptRequired(message string) (string, error) {
	for {
		input, err := Prompt(message)
		if err != nil {
			return "", err
		}
		
		if strings.TrimSpace(input) != "" {
			return input, nil
		}
		
		Error("This field is required. Please enter a value.")
	}
}

// PromptFilePath asks for a file path and validates it exists.
func PromptFilePath(message string) (string, error) {
	for {
		path, err := PromptRequired(message)
		if err != nil {
			return "", err
		}
		
		// Expand ~ to home directory
		if strings.HasPrefix(path, "~/") {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", fmt.Errorf("unable to get home directory: %w", err)
			}
			path = strings.Replace(path, "~", home, 1)
		}
		
		// Check if file exists
		if _, err := os.Stat(path); os.IsNotExist(err) {
			Error("File not found: %s. Please check the path and try again.", path)
			continue
		}
		
		return path, nil
	}
}

// PromptConfirmation asks for explicit confirmation with a specific word.
func PromptConfirmation(message, confirmWord string) (bool, error) {
	fmt.Fprintf(os.Stdout, "\n%s\n", message)
	prompt := fmt.Sprintf("Type \"%s\" to confirm, or press Enter to cancel", confirmWord)
	
	input, err := Prompt(prompt)
	if err != nil {
		return false, err
	}
	
	return strings.TrimSpace(input) == confirmWord, nil
}

