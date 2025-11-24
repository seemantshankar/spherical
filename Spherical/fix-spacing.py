#!/usr/bin/env python3
"""
Fix markdown spacing issues:
- Ensure section headers (##) are on their own lines
- Add blank lines between sections
- Fix concatenated headers
"""

import sys
import re

def fix_markdown_spacing(content):
    """Fix spacing issues in markdown."""
    
    # Fix concatenated headers (text##Header -> text\n\n##Header)
    content = re.sub(r'([a-zA-Z0-9).\]\*-])(##\s)', r'\1\n\n\2', content)
    
    # Ensure blank line before section headers (if not already)
    lines = content.split('\n')
    fixed_lines = []
    
    for i, line in enumerate(lines):
        # If this is a section header
        if line.strip().startswith('##'):
            # Check if previous line is not blank and not a header
            if i > 0 and fixed_lines and fixed_lines[-1].strip() != '':
                fixed_lines.append('')  # Add blank line
        
        fixed_lines.append(line)
    
    # Remove multiple consecutive blank lines (keep max 2)
    result = []
    blank_count = 0
    for line in fixed_lines:
        if line.strip() == '':
            blank_count += 1
            if blank_count <= 2:
                result.append(line)
        else:
            blank_count = 0
            result.append(line)
    
    return '\n'.join(result)

def main():
    if len(sys.argv) < 2:
        print("Usage: python fix-spacing.py <input-file> [output-file]")
        sys.exit(1)
    
    input_file = sys.argv[1]
    output_file = sys.argv[2] if len(sys.argv) > 2 else input_file
    
    with open(input_file, 'r', encoding='utf-8') as f:
        content = f.read()
    
    fixed = fix_markdown_spacing(content)
    
    with open(output_file, 'w', encoding='utf-8') as f:
        f.write(fixed)
    
    print(f"✓ Fixed markdown spacing")
    print(f"  Input:  {input_file}")
    print(f"  Output: {output_file}")

if __name__ == '__main__':
    main()


