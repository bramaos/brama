#!/usr/bin/env bash
# verify.sh drives the real brama binary against throwaway projects and records what it
# did. It is a driver for agents proving a change, not a test: make check never runs it.
#
#   verify.sh build                              build .scratch/verify/brama from this checkout
#   verify.sh doctor                             is the built binary worth driving?
#   verify.sh sandbox <fixture>                  make a project in $TMPDIR, print its path
#   verify.sh run <sandbox> <label> [--expect N] -- <brama args...>
#                                                run brama in the sandbox, save evidence
#   verify.sh cleanup                            remove sandboxes this script made; keep evidence
#
# Fixtures: bare, wordpress, bedrock, initialized, classified, approved.
set -euo pipefail

repo=$(git -C "$(dirname "$0")" rev-parse --show-toplevel)
state="$repo/.scratch/verify"
bin="$state/brama"
evidence_root="$state/evidence"
registry="$state/sandboxes.list"

die() { echo "verify: $*" >&2; exit 1; }

usage() {
	sed -n '2,13p' "$0" | sed 's/^# \{0,1\}//' >&2
	exit 2
}

version_stamp() { git -C "$repo" describe --tags --always --dirty 2>/dev/null || echo dev; }

cmd_build() {
	mkdir -p "$state"
	(cd "$repo" && go build -ldflags "-X main.version=$(version_stamp)" -o "$bin" ./cmd/brama)
	echo "built $bin ($(version_stamp))"
}

# newest_source prints the newest .go file under cmd/ and internal/, so doctor can tell a
# binary built before the last edit from one built after it.
newest_source() {
	find "$repo/cmd" "$repo/internal" -name '*.go' ! -name '*_test.go' -newer "$bin" -print -quit
}

cmd_doctor() {
	local ok=1
	if [[ ! -x $bin ]]; then
		echo "FAIL binary: $bin missing — run: verify.sh build"
		exit 1
	fi
	echo "ok   binary: $bin"
	local stale
	stale=$(newest_source)
	if [[ -n $stale ]]; then
		echo "FAIL fresh: ${stale#"$repo"/} is newer than the binary — run: verify.sh build"
		ok=0
	else
		echo "ok   fresh: no source newer than the binary"
	fi
	local got want
	got=$("$bin" --json version | sed -n 's/.*"version": "\(.*\)".*/\1/p')
	want=$(version_stamp)
	if [[ $got != "$want" ]]; then
		echo "FAIL version: binary says $got, checkout is $want — run: verify.sh build"
		ok=0
	else
		echo "ok   version: $got"
	fi
	local live=0
	if [[ -f $registry ]]; then
		live=$(grep -c . "$registry" || true)
	fi
	echo "info sandboxes: $live live (verify.sh cleanup removes them)"
	[[ $ok == 1 ]] || exit 1
}

wp_config() { # $1 dir
	printf "<?php\ndefine('WP_HOME', 'https://acme.local.test');\n\$table_prefix = 'wp_';\n" >"$1/wp-config.php"
}

classification() {
	cat <<'EOF'
anonymize:
  tables:
    users:
      columns:
        email:
          action: fake.email
          correlate: customer
        display_name:
          action: keep
    orders:
      columns:
        billing_email:
          action: fake.email
          correlate: customer
EOF
}

cmd_sandbox() {
	local fixture=${1:-}
	[[ -n $fixture ]] || die "sandbox needs a fixture: bare, wordpress, bedrock, initialized, classified, approved"
	[[ -x $bin ]] || die "no binary — run: verify.sh build"
	# Outside the repo: brama looks for brama.yaml at or above its directory, and a
	# sandbox must never find one that is not its own.
	local dir
	dir=$(mktemp -d "${TMPDIR:-/tmp}/brama-verify.XXXXXX")
	mkdir -p "$state"
	echo "$dir" >>"$registry"
	case $fixture in
	bare) ;;
	wordpress)
		mkdir -p "$dir/wp-content/uploads"
		wp_config "$dir"
		;;
	bedrock)
		mkdir -p "$dir/web/app/uploads" "$dir/config"
		echo "<?php" >"$dir/config/application.php"
		echo "WP_HOME='https://acme.local.test'" >"$dir/.env"
		;;
	initialized | classified | approved)
		mkdir -p "$dir/wp-content/uploads"
		wp_config "$dir"
		(cd "$dir" && "$bin" --json --non-interactive init >/dev/null) || die "init failed building the $fixture fixture"
		if [[ $fixture != initialized ]]; then
			classification >>"$dir/brama.yaml"
		fi
		if [[ $fixture == approved ]]; then
			sed -i 's|^    url: https://acme.local.test$|&\n    anonymize:\n      approved:\n        - users.display_name|' "$dir/brama.yaml"
		fi
		;;
	*)
		rmdir "$dir"
		die "unknown fixture $fixture: bare, wordpress, bedrock, initialized, classified, approved"
		;;
	esac
	echo "$dir"
}

cmd_run() {
	local dir=${1:-} label=${2:-} expect=""
	[[ -n $dir && -n $label ]] || usage
	shift 2
	if [[ ${1:-} == --expect ]]; then
		expect=$2
		shift 2
	fi
	[[ ${1:-} == -- ]] || die "put brama's arguments after --"
	shift
	[[ -d $dir ]] || die "no sandbox at $dir — make one with: verify.sh sandbox <fixture>"
	[[ -x $bin ]] || die "no binary — run: verify.sh build"

	local id out n file
	id=$(basename "$dir")
	out="$evidence_root/$id"
	mkdir -p "$out"
	n=$(find "$out" -maxdepth 1 -name '*.txt' | wc -l)
	file=$(printf '%s/%02d-%s.txt' "$out" "$((n + 1))" "$label")

	local before="" stdout stderr code
	[[ -f $dir/brama.yaml ]] && before=$(cat "$dir/brama.yaml")
	stdout=$(mktemp) stderr=$(mktemp)
	set +e
	(cd "$dir" && "$bin" "$@") >"$stdout" 2>"$stderr"
	code=$?
	set -e

	{
		echo "\$ cd $dir && brama $*"
		echo "exit: $code${expect:+ (expected $expect)}"
		echo "--- stdout"
		cat "$stdout"
		echo "--- stderr"
		cat "$stderr"
		echo "--- brama.yaml"
		if [[ -f $dir/brama.yaml ]]; then
			if [[ -z $before ]]; then
				echo "(created)"
				cat "$dir/brama.yaml"
			elif [[ $before == "$(cat "$dir/brama.yaml")" ]]; then
				echo "(unchanged)"
			else
				diff <(echo "$before") "$dir/brama.yaml" || true
			fi
		else
			echo "(absent)"
		fi
	} >"$file"
	rm -f "$stdout" "$stderr"

	cat "$file"
	echo "evidence: $file"
	if [[ -n $expect && $code != "$expect" ]]; then
		die "exit $code, expected $expect"
	fi
}

cmd_cleanup() {
	[[ -f $registry ]] || { echo "nothing to clean"; return; }
	local dir
	while IFS= read -r dir; do
		# Only what sandbox made: a brama-verify.* directory under the temp root.
		[[ $dir == "${TMPDIR:-/tmp}"/brama-verify.* ]] || continue
		rm -rf -- "$dir"
		echo "removed $dir"
	done <"$registry"
	rm -f "$registry"
	echo "evidence kept in $evidence_root"
}

case ${1:-} in
build) cmd_build ;;
doctor) cmd_doctor ;;
sandbox) shift; cmd_sandbox "$@" ;;
run) shift; cmd_run "$@" ;;
cleanup) cmd_cleanup ;;
*) usage ;;
esac
