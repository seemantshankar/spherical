#!/usr/bin/env python3
"""Enhance USP sections with premium marketing bullets derived from detected cues."""

import sys

KEYWORD_RULES = [
    {
        "check": lambda text: "gear" in text and "crystal" in text,
        "bullet": "The hand-cut Orrefors® crystal gear selector adds a jewel-like centerpiece that reminds you this is true Scandinavian luxury."
    },
    {
        "check": lambda text: "led matrix" in text or "thor" in text,
        "bullet": "Iconic Thor's Hammer LED Matrix headlamps carve out your presence with signature light signatures day or night."
    },
    {
        "check": lambda text: "bowers" in text,
        "bullet": "Concert-hall acoustics come standard thanks to the 1410W Bowers & Wilkins audio suite with 19 precisely tuned speakers."
    },
    {
        "check": lambda text: "four-c" in text and "air suspension" in text,
        "bullet": "Four-C adaptive chassis with 4-corner air suspension glides over every surface, delivering limousine-like calm."
    },
    {
        "check": lambda text: "panoramic" in text and "roof" in text,
        "bullet": "A sweeping panoramic roof floods the airy cabin with Scandinavian light for every row."
    },
    {
        "check": lambda text: "massage" in text and "seat" in text,
        "bullet": "Ventilated Nappa seats with built-in massage create a spa-like sanctuary on every journey."
    }
]

DEFAULT_BULLET = "Signature Volvo craftsmanship surrounds you with premium materials, intuitive technology, and reassuring safety leadership."


def build_usps(text_lower: str):
    bullets = []
    for rule in KEYWORD_RULES:
        if rule["check"](text_lower):
            bullets.append(rule["bullet"])
    if not bullets:
        bullets.append(DEFAULT_BULLET)
    # Ensure uniqueness and period ending
    cleaned = []
    for bullet in bullets:
        if not bullet.endswith('.'):
            bullet = bullet + '.'
        if bullet not in cleaned:
            cleaned.append(bullet)
    return cleaned[:6]


def replace_usps(lines, bullets):
    output = []
    i = 0
    while i < len(lines):
        line = lines[i]
        if line.startswith('## USPs'):
            output.append('## USPs')
            output.append('')
            for bullet in bullets:
                output.append(f'- {bullet}')
            output.append('')
            # skip old USP lines
            i += 1
            while i < len(lines) and (lines[i].strip() == '' or lines[i].startswith('-')):
                i += 1
            continue
        output.append(line)
        i += 1
    return output


def main():
    if len(sys.argv) < 2:
        print('Usage: enhance_usps.py <markdown-file>')
        return
    path = sys.argv[1]
    with open(path, 'r', encoding='utf-8') as f:
        content = f.read()
    text_lower = content.lower()
    bullets = build_usps(text_lower)
    lines = content.splitlines()
    if not any(line.startswith('## USPs') for line in lines):
        print('No USP section found; skipped.')
        return
    new_lines = replace_usps(lines, bullets)
    with open(path, 'w', encoding='utf-8') as f:
        f.write('\n'.join(new_lines).rstrip() + '\n')
    print(f'✓ Enhanced USPs in {path} with {len(bullets)} premium bullets')


if __name__ == '__main__':
    main()
