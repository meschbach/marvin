#!/usr/bin/env bash
# Wraps long lines in Markdown files at 120 characters.
# Strips trailing whitespace from all lines.
# Skips fenced code blocks, ASCII art diagrams, table rows, and indented code blocks.
set -euo pipefail

WRAP=120

strip_trailing() {
    sed -e 's/[[:space:]]*$//'
}

process_file() {
    local file="$1"
    local temp
    temp="$(mktemp)"
    local in_code=0

    while IFS= read -r line || [ -n "$line" ]; do
        # Strip trailing whitespace (fold may add it at break points)
        line="$(printf '%s' "$line" | strip_trailing)"

        # Toggle fenced code block state
        if [[ "$line" =~ ^\`\`\` ]]; then
            in_code=$((1 - in_code))
            printf '%s\n' "$line" >> "$temp"
            continue
        fi

        # Inside code block: pass through
        if [ "$in_code" -eq 1 ]; then
            printf '%s\n' "$line" >> "$temp"
            continue
        fi

        # ASCII art (box-drawing characters): pass through
        if echo "$line" | grep -qE '[─│┌┐└┘├┤┼╔╗╚╝╠╣═║]'; then
            printf '%s\n' "$line" >> "$temp"
            continue
        fi

        # Table rows (pipe-delimited, including blockquotes): pass through
        table_re='^[[:space:]>]*\|.*\|[[:space:]]*$'
        sep_re='^[[:space:]>]*\|[[:space:]]*-'
        if [[ "$line" =~ $table_re ]] || [[ "$line" =~ $sep_re ]]; then
            printf '%s\n' "$line" >> "$temp"
            continue
        fi

        # Indented code (4+ leading spaces): pass through
        if [[ "$line" =~ ^[[:space:]]{4,} ]] && [ -n "$line" ]; then
            printf '%s\n' "$line" >> "$temp"
            continue
        fi

        # Empty lines: pass through
        if [ -z "$line" ]; then
            printf '%s\n' "$line" >> "$temp"
            continue
        fi

        # Wrap long prose lines at word boundaries
        if [ "${#line}" -gt "$WRAP" ]; then
            echo "$line" | fold -s -w "$WRAP" | strip_trailing >> "$temp"
        else
            printf '%s\n' "$line" >> "$temp"
        fi
    done < "$file"

    mv "$temp" "$file"
}

for f in "$@"; do
    if [ -f "$f" ]; then
        process_file "$f"
    fi
done
