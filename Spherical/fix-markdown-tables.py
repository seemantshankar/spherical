#!/usr/bin/env python3
"""
Script to fix malformed markdown tables in extracted PDF content.
Ensures all tables have consistent 3-column structure.
"""

import sys
import re

def fix_table_row(line):
    """Fix a table row to ensure it has exactly 3 columns."""
    if not line.strip().startswith('|'):
        return line
    
    # Count pipes
    pipes = line.count('|')
    
    # Header separator line (|---|---|---|)
    if '---' in line:
        return '| Category | Specification | Value |\n|----------|---------------|-------|\n'
    
    # If it's a proper 3-column row (4 pipes: start, 2 separators, end)
    if pipes == 4:
        return line
    
    # If it has more than 4 pipes, we need to merge columns
    if pipes > 4:
        parts = [p.strip() for p in line.split('|')]
        parts = [p for p in parts if p]  # Remove empty strings
        
        if len(parts) > 3:
            # Strategy: Keep first as category, second as spec, merge rest as value
            category = parts[0] if len(parts) > 0 else ''
            specification = parts[1] if len(parts) > 1 else ''
            value = ' - '.join(parts[2:]) if len(parts) > 2 else ''
            
            return f'| {category} | {specification} | {value} |\n'
    
    return line

def fix_markdown_tables(content):
    """Process markdown content and fix table formatting."""
    lines = content.split('\n')
    fixed_lines = []
    in_table = False
    
    for i, line in enumerate(lines):
        # Detect table start
        if '|' in line and not in_table:
            # Check if next line is separator
            if i + 1 < len(lines) and '---' in lines[i + 1]:
                in_table = True
        
        # Fix table rows
        if in_table and '|' in line:
            fixed_line = fix_table_row(line)
            fixed_lines.append(fixed_line.rstrip())
            
            # Check if table ended (next line doesn't have |)
            if i + 1 < len(lines) and '|' not in lines[i + 1]:
                in_table = False
        else:
            fixed_lines.append(line)
    
    return '\n'.join(fixed_lines)

def main():
    if len(sys.argv) < 2:
        print("Usage: python fix-markdown-tables.py <input-file> [output-file]")
        print("If output-file is not specified, will overwrite input file")
        sys.exit(1)
    
    input_file = sys.argv[1]
    output_file = sys.argv[2] if len(sys.argv) > 2 else input_file
    
    # Read input
    with open(input_file, 'r', encoding='utf-8') as f:
        content = f.read()
    
    # Fix tables
    fixed_content = fix_markdown_tables(content)
    
    # Write output
    with open(output_file, 'w', encoding='utf-8') as f:
        f.write(fixed_content)
    
    print(f"✓ Fixed markdown tables")
    print(f"  Input:  {input_file}")
    print(f"  Output: {output_file}")

if __name__ == '__main__':
    main()


