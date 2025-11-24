#!/usr/bin/env python3
"""
Clean up empty sections from extracted markdown.
Removes:
- Empty specification tables (header only, no data)
- Sections with "(No ... found)" messages
- Incomplete section headers
- Multiple consecutive blank lines
"""

import sys
import re

def clean_markdown(content):
    """Remove empty sections and clean up the markdown."""
    lines = content.split('\n')
    cleaned = []
    i = 0
    
    while i < len(lines):
        line = lines[i].strip()
        
        # Skip empty specification tables
        if line.startswith('## Specifications'):
            # Look ahead to see if there's actual data
            has_data = False
            j = i + 1
            # Skip header and separator
            while j < len(lines) and j < i + 5:
                next_line = lines[j].strip()
                if next_line.startswith('|') and not next_line.startswith('|---'):
                    # Check if it's not just a header row
                    if '|' in next_line and next_line.count('|') >= 4:
                        parts = [p.strip() for p in next_line.split('|')[1:-1]]
                        if parts and not all(p in ['Category', 'Specification', 'Value', '', '---'] for p in parts):
                            has_data = True
                            break
                elif next_line and not next_line.startswith('|') and not next_line.startswith('#'):
                    break
                j += 1
            
            if not has_data:
                # Skip this entire section
                i = j
                continue
        
        # Skip sections with "no content" messages
        if '(No' in line and 'found' in line.lower():
            i += 1
            continue
        
        # Skip incomplete section headers
        if line.startswith('## ') and line.endswith('('):
            i += 1
            continue
        
        # Add the line
        cleaned.append(lines[i])
        i += 1
    
    # Remove multiple consecutive blank lines
    result = []
    prev_blank = False
    for line in cleaned:
        is_blank = not line.strip()
        if is_blank and prev_blank:
            continue
        result.append(line)
        prev_blank = is_blank
    
    # Remove leading and trailing blank lines
    while result and not result[0].strip():
        result.pop(0)
    while result and not result[-1].strip():
        result.pop()
    
    return '\n'.join(result)

def main():
    if len(sys.argv) < 2:
        print("Usage: python clean-empty-sections.py <input-file> [output-file]")
        sys.exit(1)
    
    input_file = sys.argv[1]
    output_file = sys.argv[2] if len(sys.argv) > 2 else input_file
    
    with open(input_file, 'r', encoding='utf-8') as f:
        content = f.read()
    
    cleaned = clean_markdown(content)
    
    with open(output_file, 'w', encoding='utf-8') as f:
        f.write(cleaned)
    
    print(f"✓ Cleaned empty sections from markdown")
    print(f"  Input:  {input_file}")
    print(f"  Output: {output_file}")

if __name__ == '__main__':
    main()


