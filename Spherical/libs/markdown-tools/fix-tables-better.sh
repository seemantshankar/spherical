#!/bin/bash
# Fix markdown tables with 4 columns to have 3 columns

input_file="$1"
output_file="${2:-$input_file}"

# Use sed to fix the 4-column rows by merging column 3 and 4
# Pattern: | col1 | col2 | col3 | col4 |
# Replace with: | col1 | col2 | col3 - col4 |

sed -E 's/\| ([^|]*) \| ([^|]*) \| ([^|]*) \| ([^|]*) \|/| \1 | \2 | \3 - \4 |/g' "$input_file" > "$output_file"

echo "✓ Fixed markdown tables in $output_file"
