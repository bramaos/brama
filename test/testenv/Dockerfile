# No `# syntax=` directive on purpose. Pinning the frontend makes every build resolve
# docker/dockerfile:1 from Docker Hub, which on a Docker Desktop WSL setup goes
# through the Windows credential helper and fails before a single layer is read. This
# file uses nothing the built-in frontend of Docker 23+ does not already have.

# A Server, as CONTEXT.md defines one: a registered SSH host.
#
# Ubuntu 24.04 with systemd as PID 1, a real sshd, and the stack a rented WordPress
# Server runs — MariaDB, Apache, PHP-FPM, WP-CLI, Bedrock. MariaDB is *inside* the
# image, not a sibling container, so the Shim finds `mysqldump` locally over a unix
# socket exactly as it would on a rented Server.
#
# This image is deliberately not hardened. It ships sshd, sudo, a package manager and
# a writable root filesystem, and it runs privileged so systemd can own PID 1. That is
# the opposite of what a production image should do, and it is the point: the thing
# under test is what Brama does to a Server that has all of those.
# See docs/adr/0008-test-against-containerised-servers.md.

ARG UBUNTU_VERSION=24.04
ARG COMPOSER_VERSION=2.8

# --- Composer -----------------------------------------------------------------
# Taken from the official image rather than piped from an installer script, so there
# is no `curl | php` in the build and the version is a tag rather than whatever the
# installer served today.
FROM composer:${COMPOSER_VERSION} AS composer

# --- Base: operating system, systemd, sshd, LAMP -------------------------------
#
# Split from the server stage on purpose: this layer is minutes of apt and is
# identical for every Server, while the stage below is seconds of configuration that
# changes whenever the rig does.
FROM ubuntu:${UBUNTU_VERSION} AS base

ARG MARIADB_SERIES=11.4
ARG PHP_VERSION=8.3
ARG WP_CLI_VERSION=2.12.0

ENV DEBIAN_FRONTEND=noninteractive \
    container=docker \
    LC_ALL=C.UTF-8 \
    LANG=C.UTF-8

# Stop the MariaDB and Apache postinst scripts from trying to start a service: there
# is no init running during a build, and their failure would fail the layer. Removed
# again in the final stage, so the running Server behaves like any other.
#
# docker-clean makes apt delete its own downloads, which defeats the cache mounts
# below.
RUN printf '#!/bin/sh\nexit 101\n' > /usr/sbin/policy-rc.d \
 && chmod 0755 /usr/sbin/policy-rc.d \
 && rm -f /etc/apt/apt.conf.d/docker-clean

RUN --mount=type=cache,target=/var/cache/apt,sharing=locked \
    --mount=type=cache,target=/var/lib/apt/lists,sharing=locked \
    apt-get update \
 && apt-get install -y --no-install-recommends \
      ca-certificates curl gnupg \
      systemd systemd-sysv dbus \
      openssh-server sudo \
      less nano procps iproute2 unzip rsync

# MariaDB from upstream rather than Ubuntu's 10.11: a rented Server that matters is
# generally on the current LTS, and the series is pinned so a rebuild in six months
# is still the same major.minor. archive.mariadb.org is immutable; dlm.mariadb.com
# redirects to whichever mirror answered today.
RUN curl -fsSL -o /etc/apt/keyrings/mariadb.asc \
      https://mariadb.org/mariadb_release_signing_key.pgp \
 && printf '%s\n' \
      "deb [signed-by=/etc/apt/keyrings/mariadb.asc] https://archive.mariadb.org/mariadb-${MARIADB_SERIES}/repo/ubuntu noble main" \
      > /etc/apt/sources.list.d/mariadb.list

RUN --mount=type=cache,target=/var/cache/apt,sharing=locked \
    --mount=type=cache,target=/var/lib/apt/lists,sharing=locked \
    apt-get update \
 && apt-get install -y --no-install-recommends \
      mariadb-server mariadb-client \
      apache2 \
      php${PHP_VERSION}-fpm php${PHP_VERSION}-cli \
      php${PHP_VERSION}-mysql php${PHP_VERSION}-xml php${PHP_VERSION}-curl \
      php${PHP_VERSION}-mbstring php${PHP_VERSION}-gd php${PHP_VERSION}-zip \
      php${PHP_VERSION}-intl php${PHP_VERSION}-bcmath

# WP-CLI: downloaded, checksummed, then installed — three steps rather than one pipe
# into a shell, so a corrupted or substituted phar fails the build instead of running.
RUN curl -fsSL -o /tmp/wp.phar \
      "https://github.com/wp-cli/wp-cli/releases/download/v${WP_CLI_VERSION}/wp-cli-${WP_CLI_VERSION}.phar" \
 && curl -fsSL -o /tmp/wp.phar.sha512 \
      "https://github.com/wp-cli/wp-cli/releases/download/v${WP_CLI_VERSION}/wp-cli-${WP_CLI_VERSION}.phar.sha512" \
 && echo "$(cat /tmp/wp.phar.sha512)  /tmp/wp.phar" | sha512sum -c - \
 && install -m 0755 /tmp/wp.phar /usr/local/bin/wp \
 && rm -f /tmp/wp.phar /tmp/wp.phar.sha512 \
 && wp --allow-root --version

COPY --from=composer /usr/bin/composer /usr/local/bin/composer

# --- Server: users, services, site ---------------------------------------------
FROM base AS server

ARG PHP_VERSION=8.3
ARG BEDROCK_VERSION=1.31.4
ARG WORDPRESS_VERSION=7.1
ARG WOOCOMMERCE_VERSION=11.1.0

ENV BRAMA_TESTENV_SITE=/srv/www/bedrock \
    COMPOSER_ALLOW_SUPERUSER=1

# The login user is `deploy`, not root: the Shim installs under a normal home
# directory, and the non-root permission path is what gets exercised. Ubuntu's stock
# uid-1000 `ubuntu` account is removed so deploy takes 1000, as the first account on
# a freshly rented Server would.
RUN userdel -r ubuntu 2>/dev/null || true \
 && useradd --create-home --uid 1000 --shell /bin/bash --groups www-data deploy \
 && install -d -o deploy -g deploy -m 0700 /home/deploy/.ssh \
 && printf 'deploy ALL=(ALL) NOPASSWD:ALL\n' > /etc/sudoers.d/90-deploy \
 && chmod 0440 /etc/sudoers.d/90-deploy \
 && visudo -cf /etc/sudoers.d/90-deploy

# Bedrock, from the tagged template, with core and WooCommerce pinned. Composer
# resolves the rest: the determinism the tests rely on lives in the seeded data, not
# in the dependency graph, and pinning every transitive package would make the image
# unbuildable the first time one of them is yanked.
RUN --mount=type=cache,target=/root/.composer/cache \
    install -d /srv/www \
 && composer create-project "roots/bedrock:${BEDROCK_VERSION}" "${BRAMA_TESTENV_SITE}" \
      --no-interaction --no-progress \
 && cd "${BRAMA_TESTENV_SITE}" \
 && composer require "roots/wordpress:${WORDPRESS_VERSION}" --no-interaction --no-progress

RUN curl -fsSL -o /tmp/woocommerce.zip \
      "https://downloads.wordpress.org/plugin/woocommerce.${WOOCOMMERCE_VERSION}.zip" \
 && unzip -q /tmp/woocommerce.zip -d "${BRAMA_TESTENV_SITE}/web/app/plugins" \
 && rm -f /tmp/woocommerce.zip \
 && install -d "${BRAMA_TESTENV_SITE}/web/app/uploads" \
 && chown -R deploy:www-data "${BRAMA_TESTENV_SITE}" \
 && chmod 2775 "${BRAMA_TESTENV_SITE}/web/app/uploads"

COPY rootfs/ /
COPY seed/ /usr/local/share/brama-testenv/seed/
RUN chmod 0755 /usr/local/sbin/brama-testenv-* /usr/local/share/brama-testenv/seed/*.sh

RUN a2enmod proxy proxy_fcgi setenvif rewrite \
 && a2enconf "php${PHP_VERSION}-fpm" \
 && a2dissite 000-default \
 && a2ensite brama-testenv

# systemd in a container: drop every unit the default targets pull in, then enable
# back exactly the ones a Server needs. Pruning and re-enabling beats masking a list
# of known-hostile units, because such a list is only complete until the next Ubuntu
# point release adds to it — and one unit left running that cannot work in a
# container turns `systemctl is-system-running` into `degraded`.
RUN find /etc/systemd/system /lib/systemd/system -path '*.wants/*' \
      -not -name '*journald*' \
      -not -name '*systemd-tmpfiles*' \
      -not -name '*systemd-user-sessions*' \
      -delete \
 && systemctl enable \
      dbus.service \
      ssh.service \
      mariadb.service \
      apache2.service \
      "php${PHP_VERSION}-fpm.service" \
      brama-testenv-authkeys.service \
      brama-testenv-provision.service \
 && rm -f /usr/sbin/policy-rc.d

EXPOSE 22 80

# systemd's shutdown signal. Without it Docker sends SIGTERM, which systemd reads as
# "isolate rescue.target", and the container takes the full stop grace period to die.
STOPSIGNAL SIGRTMIN+3

# A Server is ready when systemd says the whole boot is — `is-system-running` exits
# non-zero for `degraded`, which is exactly the state an acceptance criterion forbids
# — and when the services the rig exists to reach are up.
HEALTHCHECK --interval=5s --timeout=5s --retries=40 --start-period=20s \
  CMD systemctl is-system-running --quiet \
      && systemctl is-active --quiet ssh \
      && systemctl is-active --quiet mariadb \
      && systemctl is-active --quiet apache2

CMD ["/sbin/init"]
