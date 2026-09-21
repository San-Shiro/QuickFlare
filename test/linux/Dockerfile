# Linux test environment for the QuickFlare CLI.
#
# The CLI is developed on Windows and cross-compiled, so nothing about its
# Linux behaviour is exercised by building it. This image runs it on a real
# Linux userland with systemd present, which is where the interesting failures
# are: XDG path resolution, 0600 permissions, and a systemd unit that is only
# ever validated by systemd itself.
#
# systemd is installed but NOT run as PID 1. Running it properly needs
# --privileged and cgroup mounts, and `systemctl --user` needs a login session
# on top of that - a lot of moving parts for a check that `systemd-analyze
# verify` already does better, because it reads the unit the way the parser
# does rather than the way a happy path does.
#
# /run/systemd/system is created so the CLI takes its systemd code path and
# actually writes a unit for that verifier to read. The subsequent
# `systemctl --user` call still fails, and is expected to: there is no session
# bus. What is under test is the unit, not the handshake with a running
# systemd, which a container cannot honestly provide.
FROM ubuntu:24.04

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y --no-install-recommends \
        systemd \
        ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# A normal unprivileged user. Running as root would hide exactly the
# permission behaviour worth checking, and would make the 0600 assertions
# meaningless.
RUN useradd --create-home --shell /bin/bash tester

# See the note above: this makes systemdPresent() true so install proceeds as
# far as writing the unit file.
RUN mkdir -p /run/systemd/system

COPY --chown=root:root quickflare /usr/local/bin/quickflare
RUN chmod 0755 /usr/local/bin/quickflare

COPY --chown=root:root run-tests.sh /usr/local/bin/run-tests.sh
RUN chmod 0755 /usr/local/bin/run-tests.sh

USER tester
WORKDIR /home/tester
ENTRYPOINT ["/usr/local/bin/run-tests.sh"]
