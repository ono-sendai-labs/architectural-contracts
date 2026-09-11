# Shared by the native selfcheck recipe and its regression test. The caller
# supplies the binary and map so the same checks work in a temporary copied Go
# tree without relying on ambient sibling artifacts.

selfcheck_stage_component() {
	if (($# < 3 || $# > 4)); then
		echo "selfcheck staging: expected arcc binary, map, manifest, and optional expected verdict" >&2
		return 2
	fi

	local arcc_bin="$1"
	local map_path="$2"
	local manifest="$3"
	local expected="${4:-pass}"
	local base="${manifest%.textproto}"
	local component="${manifest%/component.textproto}"
	component=${component##*/}

	case "$expected" in
	pass|fail)	;;
	*)
		echo "selfcheck staging verdict for component $component: expected verdict must be pass or fail, got $expected" >&2
		return 2
		;;
	esac

	if ! "$arcc_bin" check "$manifest" --stdlib-map="$map_path" \
		--report-out="$base.report.json" --surface-out="$base.surface.json" \
		--report-verdict-only >/dev/null; then
		echo "selfcheck staging analysis failed for component $component" >&2
		return 1
	fi

	# `--report-verdict-only` deliberately makes analysis violations non-fatal;
	# decode the canonical artifact through `arcc verdict` so missing, malformed,
	# contradictory, and unexpected reports all fail closed here (design R8).
	if ! "$arcc_bin" verdict "$base.report.json" --expect="$expected" >/dev/null; then
		echo "selfcheck staging verdict failed for component $component: expected $expected" >&2
		return 1
	fi
}

selfcheck_asserted_component() {
	if (($# != 1)); then
		echo "selfcheck asserted staging: expected exactly one manifest" >&2
		return 2
	fi

	local manifest="$1"
	local base="${manifest%.textproto}"
	local component="${manifest%/component.textproto}"
	component=${component##*/}

	if [[ ! -f "$base.surface.json" ]]; then
		echo "selfcheck asserted staging failed for component $component: missing native asserted surface $base.surface.json" >&2
		return 1
	fi
	if [[ -e "$base.report.json" ]]; then
		echo "selfcheck asserted staging failed for component $component: asserted component unexpectedly has a report $base.report.json" >&2
		return 1
	fi
}
