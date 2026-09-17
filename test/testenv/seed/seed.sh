#!/bin/bash
# Load the Server's deterministic WordPress + WooCommerce data.
#
# Also the reset. It drops the database before it does anything else, so a Server that
# has been poked at by hand comes back to exactly the state the tests expect without
# a rebuild — which is the difference between a rig a developer will experiment on
# and one they will be afraid to touch.
#
# Run from the host with `make testenv-seed`, or inside a Server as root.

set -euo pipefail

# shellcheck source=../rootfs/usr/local/lib/brama-testenv/container-env.sh
. /usr/local/lib/brama-testenv/container-env.sh
load_container_env

readonly SITE="${BRAMA_TESTENV_SITE:?BRAMA_TESTENV_SITE is required}"
readonly ROLE="${BRAMA_TESTENV_ROLE:?BRAMA_TESTENV_ROLE is required}"
readonly URL="${BRAMA_TESTENV_URL:?BRAMA_TESTENV_URL is required}"

# Staging is seeded smaller than production on purpose: a Pull needs a target that
# already holds real rows, so that overwriting them is something a Recovery point can
# be proved against. An empty staging would prove nothing.
case "${ROLE}" in
production) readonly USERS=60 PRODUCTS=40 ORDERS=25 ;;
staging) readonly USERS=12 PRODUCTS=8 ORDERS=5 ;;
*)
	echo "unknown role ${ROLE}: expected production or staging" >&2
	exit 1
	;;
esac

# Everything runs as deploy, from the site root — the same user and directory the
# Shim will be working in.
wp() {
	runuser -u deploy -- /usr/local/bin/wp --path="${SITE}/web/wp" "$@"
}

cd "${SITE}"

echo "  resetting the database"
wp db reset --yes

echo "  installing WordPress"
# Fixed credentials, and they are not a secret: the Server listens on the loopback
# interface of one laptop and holds nothing but the rows generated below.
wp core install \
	--url="${URL}" \
	--title="Brama testenv (${ROLE})" \
	--admin_user=admin \
	--admin_email="admin@brama-testenv.invalid" \
	--admin_password=brama-testenv \
	--skip-email

wp option update timezone_string UTC
wp option update date_format 'Y-m-d'
wp rewrite structure '/%postname%/' --hard

echo "  activating WooCommerce"
wp plugin activate woocommerce
# WooCommerce asks to run a setup wizard on first activation and its notices change
# what the options table holds; turning them off keeps two seedings byte-comparable.
wp option update woocommerce_onboarding_profile '{"skipped":true}' --format=json
wp option delete _transient_wc_onboarding_profile 2>/dev/null || true

echo "  generating ${USERS} users, ${PRODUCTS} products, ${ORDERS} orders"
wp eval-file /usr/local/share/brama-testenv/seed/seed.php \
	"${USERS}" "${PRODUCTS}" "${ORDERS}"

echo "  seeded ${ROLE}"
