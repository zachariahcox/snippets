#!/usr/bin/env bash
# Prints the largest existing vMAJOR.MINOR.PATCH tag and the suggested next tag for a bump kind.
# Usage: next-tag.sh major|minor|patch
# Does not run git; only echoes the git tag command to run.
set -euo pipefail

bump="${1:-}"
if [[ "$bump" != "major" && "$bump" != "minor" && "$bump" != "patch" ]]; then
	echo "usage: $0 major|minor|patch" >&2
	exit 1
fi

best_maj=-1
best_min=-1
best_pat=-1

while IFS= read -r t; do
	[[ -n "$t" ]] || continue
	s="${t#v}"
	if [[ "$s" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
		IFS=. read -r maj min pat <<<"$s"
		if ((maj > best_maj || (maj == best_maj && min > best_min) || (maj == best_maj && min == best_min && pat > best_pat))); then
			best_maj=$maj
			best_min=$min
			best_pat=$pat
		fi
	fi
done < <(git tag 2>/dev/null || true)

if ((best_maj < 0)); then
	echo "No existing semantic tags (vMAJOR.MINOR.PATCH with integer parts)."
	case "$bump" in
	major) nmaj=1 nmin=0 npat=0 ;;
	minor) nmaj=0 nmin=1 npat=0 ;;
	patch) nmaj=0 nmin=0 npat=1 ;;
	esac
else
	echo "Largest semantic tag: v${best_maj}.${best_min}.${best_pat}"
	case "$bump" in
	patch) nmaj=$best_maj nmin=$best_min npat=$((best_pat + 1)) ;;
	minor) nmaj=$best_maj nmin=$((best_min + 1)) npat=0 ;;
	major) nmaj=$((best_maj + 1)) nmin=0 npat=0 ;;
	esac
fi

next="v${nmaj}.${nmin}.${npat}"
echo "Suggested next (${bump}): ${next}"
echo "git tag -a \"${next}\" -m \"Release ${next}\""
echo "git push origin \"${next}\""
