# Read the container's environment into this shell.
#
# systemd does not hand its own inherited environment to the services it starts, so a
# unit cannot see what `docker compose` set in `environment:`. PID 1 can, and its
# block is readable at /proc/1/environ — NUL-separated, which is why this cannot be a
# `source`.
#
# Only BRAMA_TESTENV_* is imported. The rest of PID 1's environment is systemd's
# business, and overwriting PATH or HOME from it is how a boot script starts
# behaving differently depending on how the container was launched.
load_container_env() {
	local entry
	while IFS= read -r -d '' entry; do
		case "${entry}" in
		BRAMA_TESTENV_*=*) export "${entry?}" ;;
		esac
	done < /proc/1/environ
}
