#!/usr/bin/env python3
"""
Fix 4-column markdown tables to 3-column format.
Merges the last two columns with a hyphen separator.
"""

import sys
import re

def fix_table_line(line):
    """Fix a table line that has 4 columns instead of 3."""
    # Match lines like: | col1 | col2 | col3 | col4 |
    # We need to count actual columns (content between pipes)
    
    if not line.strip().startswith('|'):
        return line
    
    # Split by pipe and filter out empty strings from start/end
    parts = line.split('|')
    parts = [p.strip() for p in parts[1:-1]]  # Remove first and last empty elements
    
    # If we have exactly 4 columns, merge last two
    if len(parts) == 4:
        col1, col2, col3, col4 = parts
        # Merge col3 and col4 with a dash
        merged_value = f"{col3} - {col4}" if col3 and col4 else (col3 or col4)
        return f"| {col1} | {col2} | {merged_value} |\n"
    
    # Otherwise return as-is
    return line

def main():
    if len(sys.argv) < 2:
        print("Usage: python fix-4col-tables.py <input-file> [output-file]")
        sys.exit(1)
    
    input_file = sys.argv[1]
    output_file = sys.argv[2] if len(sys.argv) > 2 else input_file + ".fixed"
    
    with open(input_file, 'r', encoding='utf-8') as f:
        lines = f.readlines()
    
    fixed_lines = [fix_table_line(line) for line in lines]
    
    with open(output_file, 'w', encoding='utf-8') as f:
        f.writelines(fixed_lines)
    
    print(f"✓ Fixed 4-column tables")
    print(f"  Input:  {input_file}")
    print(f"  Output: {output_file}")

if __name__ == '__main__':
    main()





