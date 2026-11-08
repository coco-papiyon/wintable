#!/bin/sh
set -eu
test -s /home/runner/.ssh/authorized_keys
chown runner:runner /home/runner/.ssh/authorized_keys
chmod 0600 /home/runner/.ssh/authorized_keys
ssh-keygen -q -t ed25519 -N '' -f /run/sshd/ssh_host_ed25519_key
/usr/sbin/sshd -t
exec /usr/sbin/sshd -D -e
