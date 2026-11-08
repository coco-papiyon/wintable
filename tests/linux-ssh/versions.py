"""Management-only inventory. No environment or credential contents."""
import json
import os
import platform
import subprocess
import sys

packages = subprocess.check_output(
    ["dpkg-query", "-W", "-f=${Package}=${Version}\n"], text=True
).splitlines()
print(json.dumps({
    "packages": sorted(packages), "python": platform.python_version(),
    "sshd": subprocess.run(["/usr/sbin/sshd", "-V"], capture_output=True,
                           text=True, check=True).stderr.strip(),
    "os_release": open("/etc/os-release").read(),
    "arch": platform.machine(), "kernel": platform.release(),
    "implementation": sys.implementation.name,
    "shell": os.readlink("/bin/sh"),
}))
