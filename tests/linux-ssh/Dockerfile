FROM debian:13-slim@sha256:7378260c0a098d27ac206225db5e7d92b8c7c84ea7afccc86eeb513756bbd392
SHELL ["/bin/sh", "-ec"]
# Immutable archive, signature verification retained; no rolling mirror fallback.
RUN rm -f /etc/apt/sources.list.d/*; \
    printf '%s\n' 'deb [check-valid-until=no] http://snapshot.debian.org/archive/debian/20261105T000000Z/ trixie main' 'deb [check-valid-until=no] http://snapshot.debian.org/archive/debian-security/20261105T000000Z/ trixie-security main' > /etc/apt/sources.list
COPY packages.install /tmp/packages.install
RUN apt-get update; \
    DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends $(tr -d '\r' < /tmp/packages.install); \
    rm -rf /var/lib/apt/lists/* /etc/ssh/ssh_host_* /tmp/packages.install; \
    useradd --create-home --shell /bin/sh runner; \
    usermod --password '*' runner; \
    install -d -m 0755 /run/sshd /fixture; \
    install -d -o runner -g runner -m 0700 /home/runner/.ssh /fixture/results
COPY sshd_config /etc/ssh/sshd_config
COPY probe.py versions.py start.sh /fixture/
RUN chmod 0555 /fixture/start.sh /fixture/probe.py /fixture/versions.py
EXPOSE 22
ENTRYPOINT ["/bin/sh", "/fixture/start.sh"]
