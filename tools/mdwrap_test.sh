#!/usr/bin/env bash
# Test suite for tools/mdwrap.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
MDWRAP="$SCRIPT_DIR/mdwrap.sh"
BATS_PREFIX=""
test_count=0

pass=0
fail=0

setup() {
    test_dir="$(mktemp -d)"
}

teardown() {
    rm -rf "$test_dir"
}

assert_eq() {
    local expected="$1"
    local actual="$2"
    local label="$3"
    test_count=$((test_count + 1))
    if [ "$expected" = "$actual" ]; then
        echo "  PASS: $label"
        pass=$((pass + 1))
    else
        echo "  FAIL: $label"
        echo "    Expected: |$expected|"
        echo "    Actual:   |$actual|"
        fail=$((fail + 1))
    fi
}

# Write content to a file in test dir, run mdwrap on it, output result
run_wrap() {
    local content="$1"
    printf '%s\n' "$content" > "$test_dir/input.md"
    bash "$MDWRAP" "$test_dir/input.md"
    cat "$test_dir/input.md"
}

# ---- Tests ----

echo "=== mdwrap.sh test suite ==="
echo ""

# 1. Basic wrapping
echo "--- Basic wrapping ---"
setup
input="$(python3 -c "print('x' * 121)")"
result="$(run_wrap "$input")"
actual_len="$(echo "$result" | wc -l | tr -d ' ')"
assert_eq "2" "$actual_len" "Long line wraps to 2 lines"
teardown

# 2. Short line preserved
echo "--- Short line preserved ---"
setup
input="$(python3 -c "print('x' * 119)")"
result="$(run_wrap "$input")"
actual_len="$(echo "$result" | wc -l | tr -d ' ')"
assert_eq "1" "$actual_len" "119-char line stays as 1 line"
teardown

# 3. 120-char line boundary
echo "--- Boundary test ---"
setup
input="$(python3 -c "print('x' * 120)")"
result="$(run_wrap "$input")"
actual_len="$(echo "$result" | wc -l | tr -d ' ')"
assert_eq "1" "$actual_len" "120-char line stays as 1 line (exactly at limit)"
teardown

# 4. Trailing whitespace stripped from content
echo "--- Trailing whitespace ---"
setup
input="hello  "
result="$(run_wrap "$input")"
assert_eq "hello" "$result" "Trailing spaces stripped from short line"

# Trailing whitespace should not cause false wrap: "x * 118 + yy" = 120, under limit
input2="$(python3 -c "print('x' * 118 + 'yy')")"
result2="$(run_wrap "$input2")"
actual_len2="$(echo "$result2" | wc -l | tr -d ' ')"
assert_eq "1" "$actual_len2" "120-char line without trailing spaces stays 1 line"
teardown

# 5. Fenced code blocks preserved
echo "--- Fenced code blocks ---"
setup
input='Before

```go
'"$(python3 -c "print('x' * 200)")"'
```

After'
result="$(run_wrap "$input")"
inside="$(echo "$result" | sed -n '/^```/,/^```/p')"
# Count lines inside fenced block (should be 3: ```, content, ```)
inside_lines="$(echo "$inside" | wc -l | tr -d ' ')"
assert_eq "3" "$inside_lines" "Fenced code block has 3 lines"
# The content line should be exactly 200 chars
code_line="$(echo "$result" | sed -n '4p')"
assert_eq "200" "${#code_line}" "200-char line inside code block preserved"
teardown

# 6. ASCII art preserved
echo "--- ASCII art ---"
setup
input="┌$(python3 -c "print('─' * 150)")┐"
result="$(run_wrap "$input")"
result_len="${#result}"
assert_eq "152" "$result_len" "ASCII art line preserved (no wrap)"
teardown

# 7. Table rows preserved
echo "--- Table rows ---"
setup
input='| col1 | '"$(python3 -c "print('x' * 200)")"' | col3 |'
result="$(run_wrap "$input")"
assert_eq "1" "$(echo "$result" | wc -l | tr -d ' ')" "Table row preserved as 1 line"
teardown

# 8. Table separator rows preserved
echo "--- Table separators ---"
setup
input='|---'"$(python3 -c "print('-' * 200)")"'---|'
result="$(run_wrap "$input")"
assert_eq "1" "$(echo "$result" | wc -l | tr -d ' ')" "Table separator preserved as 1 line"
teardown

# 9. Blockquote tables preserved
echo "--- Blockquote tables ---"
setup
input='>| col1 | '"$(python3 -c "print('x' * 200)")"' | col3 |'
result="$(run_wrap "$input")"
assert_eq "1" "$(echo "$result" | wc -l | tr -d ' ')" "Blockquote table row preserved"
teardown

# 10. Indented code preserved
echo "--- Indented code ---"
setup
input='    '"$(python3 -c "print('x' * 200)")"
result="$(run_wrap "$input")"
assert_eq "1" "$(echo "$result" | wc -l | tr -d ' ')" "Indented code preserved as 1 line"
teardown

# 11. Empty lines preserved
echo "--- Empty lines ---"
setup
input="first"$'\n\n\n'"last"
result="$(run_wrap "$input")"
assert_eq "4" "$(echo "$result" | wc -l | tr -d ' ')" "4 lines (first + 2 blank + last)"
teardown

# 12. Idempotency
echo "--- Idempotency ---"
setup
input="$(python3 -c "print('A ' + 'long ' * 25 + 'line')")"
printf '%s\n' "$input" > "$test_dir/input.md"
bash "$MDWRAP" "$test_dir/input.md"
first_pass="$(cat "$test_dir/input.md")"
bash "$MDWRAP" "$test_dir/input.md"
second_pass="$(cat "$test_dir/input.md")"
assert_eq "$first_pass" "$second_pass" "Second run produces identical output"
teardown

# 13. No trailing whitespace anywhere in output (fold adds it at break points)
echo "--- No trailing whitespace ---"
setup
# Long text that will wrap and have trailing spaces at break points
input="$(python3 -c "
words = ' '.join(['word'] * 60)  # ~300 chars
print(words)
")"
result="$(run_wrap "$input")"
trailing="$(echo "$result" | grep -cE '[[:space:]]$' || true)"
assert_eq "0" "$trailing" "No trailing whitespace in any output line"
teardown

# 14. Prose with multiple long paragraphs separated by blank line
echo "--- Multiple paragraphs ---"
setup
line1="$(python3 -c "print('A' * 200)")"
line2="$(python3 -c "print('B' * 200)")"
input="${line1}"$'\n\n'"${line2}"
result="$(run_wrap "$input")"
line_count="$(echo "$result" | wc -l | tr -d ' ')"
# Each 200-char paragraph wraps to 2 lines (119 + 81), plus blank = 5
assert_eq "5" "$line_count" "Two paragraphs + blank line = 5 total lines"
# Check total chars preserved: 200 + 200 = 400 chars of content across 4 non-blank lines
content_chars="$(echo "$result" | grep -v '^$' | tr -d '\n' | wc -c | tr -d ' ')"
assert_eq "400" "$content_chars" "Total content characters preserved"
teardown

# 15. Word boundary wrapping
echo "--- Word boundary wrapping ---"
setup
# A line where last space before column 120 is at position 119
# 119 a's = 119, then ' xyz' = 4, total = 123 chars
# Last space in first 120 cols is at position 119 (between a's and xyz)
input="$(python3 -c "print('a' * 119 + ' xyz')")"
result="$(run_wrap "$input")"
lines="$(echo "$result" | wc -l | tr -d ' ')"
assert_eq "2" "$lines" "Long line wraps to 2 lines"
# First line should be 119 chars (no trailing space)
line1_chars="$(echo "$result" | head -1 | tr -d '\n' | wc -c | tr -d ' ')"
assert_eq "119" "$line1_chars" "Line 1 is 119 chars (a's only)"
# Second line should have "xyz" (3 chars)
line2_chars="$(echo "$result" | tail -1 | tr -d '\n' | wc -c | tr -d ' ')"
assert_eq "3" "$line2_chars" "Line 2 is 'xyz'"
teardown

# 16. Mixed content end-to-end
echo "--- Mixed content ---"
setup
long_prose="$(python3 -c "print('A' * 200)")"
long_code="$(python3 -c "print('B' * 200)")"
long_ascii="┌$(python3 -c "print('─' * 130)")┐"
long_table="| $(python3 -c "print('C' * 130)") |"
short_line="Normal text."
cat > "$test_dir/input.md" << EOF
# Test

$long_prose

\`\`\`
$long_code
\`\`\`

$long_ascii

$long_table

$short_line
EOF
bash "$MDWRAP" "$test_dir/input.md"
result="$(cat "$test_dir/input.md")"

# Prose line (200 A's) should wrap to multiple lines
# Find lines with A's - there should be exactly 2 (wrapped parts)
prose_count="$(echo "$result" | grep -c 'A' || true)"
assert_eq "2" "$prose_count" "200-char prose line wraps to 2 lines"

# Code block content (200 B's) should be preserved as a single line
code_line="$(echo "$result" | grep 'B' | head -1)"
assert_eq "200" "${#code_line}" "Code block content preserved (200 chars)"

# ASCII art (box-drawing chars) should be preserved as a single line
ascii_line="$(echo "$result" | grep '┌' | head -1)"
assert_eq "132" "${#ascii_line}" "ASCII art preserved (132 chars)"

# Table row should be preserved as a single line
table_line="$(echo "$result" | grep 'C' | head -1)"
# The table line is the one that has C's AND pipe characters
if echo "$table_line" | grep -q '|'; then
    assert_eq "1" "1" "Table row preserved with pipe chars"
else
    assert_eq "1" "0" "Table row preserved with pipe chars"
fi

# Short line should be preserved
assert_eq "1" "$(echo "$result" | grep -c 'Normal text\.')" "Short line preserved verbatim"

# No trailing whitespace in any line
trailing="$(echo "$result" | grep -cE '[[:space:]]$' || true)"
assert_eq "0" "$trailing" "No trailing whitespace in mixed content"
teardown

# 17. EOF without trailing newline (closing ``` preserved)
echo "--- EOF without newline ---"
setup
# File that ends with ``` without a trailing newline
printf 'test\n\n```\ncode\n```' > "$test_dir/input.md"
bash "$MDWRAP" "$test_dir/input.md"
result="$(cat "$test_dir/input.md")"
# Verify closing ``` is present
last_line="$(echo "$result" | tail -1)"
assert_eq '```' "$last_line" 'Closing ``` preserved when file ends without newline'
bt_count=$(grep -c '^```' "$test_dir/input.md" || true)
assert_eq "2" "$bt_count" 'Both ``` delimiters present'
teardown

# 18. Pre-existing code block integrity
echo "--- Code block integrity ---"
setup
# Verify all tracked md files have even number of code block delimiters
bad_files=""
cd "$SCRIPT_DIR/.."
for f in $(git ls-files '*.md'); do
  n=$(grep -c '^```' "$f" 2>/dev/null || true)
  if [ -n "$n" ] && [ "$n" -gt 0 ] 2>/dev/null; then
    r=$((n % 2))
    if [ "$r" -ne 0 ]; then
      bad_files="$bad_files $f($n)"
    fi
  fi
done
if [ -z "$bad_files" ]; then
  assert_eq "0" "0" "All tracked .md files have matching code block delimiters"
else
  assert_eq "0" "1" "All tracked .md files have matching code block delimiters (bad:$bad_files)"
fi
teardown

# ---- Summary ----
echo ""
echo "=== Results: $pass passed, $fail failed ($test_count total) ==="
if [ "$fail" -gt 0 ]; then
    exit 1
fi
