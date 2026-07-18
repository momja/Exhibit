#!/bin/sh
# tk-ready-complexity.sh — group `tk ready` output by complexity rating.
#
# Ratings live in each ticket's body as a "## Complexity" section whose first
# non-blank line is one of: XS S M L XL (see TICKETS.md). Tickets without a
# rating are grouped under "?".
#
# Usage:
#   scripts/tk-ready-complexity.sh            # all ready tickets, grouped
#   scripts/tk-ready-complexity.sh XS S       # only these levels (no headers)
set -eu

cd "$(git rev-parse --show-toplevel)"

# Note: no `case` inside $(...) — bash 3.2 (macOS /bin/sh) mis-parses it.
# awk validates the level and falls back to "?" for anything unrecognized.
rows=$(tk ready | while IFS= read -r line; do
	id=$(printf '%s' "$line" | awk '{print $1}')
	level=$(awk '
		/^## Complexity/ {f=1; next}
		f && NF {
			l = toupper($1)
			print (l == "XS" || l == "S" || l == "M" || l == "L" || l == "XL") ? l : "?"
			exit
		}' ".tickets/$id.md" 2>/dev/null || true)
	: "${level:=?}"
	printf '%s\t%s\n' "$level" "$line"
done)

if [ $# -gt 0 ]; then
	# Filter mode: print matching rows in the order the levels were given.
	for lv in "$@"; do
		ulv=$(printf '%s' "$lv" | tr '[:lower:]' '[:upper:]')
		printf '%s\n' "$rows" | awk -F '\t' -v l="$ulv" '$1 == l {print $2}'
	done
else
	for ulv in XS S M L XL "?"; do
		group=$(printf '%s\n' "$rows" | awk -F '\t' -v l="$ulv" '$1 == l {print $2}')
		if [ -n "$group" ]; then
			printf '\n== %s ==\n%s\n' "$ulv" "$group"
		fi
	done
fi
