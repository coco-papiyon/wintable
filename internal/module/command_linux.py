"""Fixed isolated Linux command receiver. Task values arrive only on stdin."""
import base64
import json
import platform
import selectors
import subprocess
import sys
import unicodedata


def reject():
    raise ValueError()


def obj(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            reject()
        result[key] = value
    return result


def text(value):
    if not isinstance(value, str) or "\0" in value:
        reject()
    data = value.encode("utf-8", "strict")
    if len(data) > 32768:
        reject()
    return len(data) + 1


def run():
    try:
        if sys.platform != "linux" or platform.python_implementation() != "CPython" or sys.version_info[:2] != (3, 13):
            reject()
        raw = sys.stdin.buffer.read(2097153)
        if len(raw) > 2097152:
            reject()
        request = json.loads(raw.decode("utf-8", "strict"), object_pairs_hook=obj,
                             parse_constant=lambda _: reject())
        if not isinstance(request, dict) or set(request) != {"module", "args"} or request["module"] != "command":
            reject()
        args = request["args"]
        if not isinstance(args, dict) or set(args) != {"executable", "argv"}:
            reject()
        exe, argv = args["executable"], args["argv"]
        total = text(exe)
        if not exe.startswith("/") or exe.endswith("/") or any(unicodedata.category(c) == "Cc" for c in exe) or any(p in (".", "..") for p in exe.split("/")):
            reject()
        if not isinstance(argv, list) or len(argv) > 4096:
            reject()
        for arg in argv:
            total += text(arg)
        if total > 65536:
            reject()
    except Exception:
        return {"error": "invalid_request"}
    try:
        child = subprocess.Popen([exe] + argv, shell=False, stdin=subprocess.DEVNULL,
                                 stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    except Exception:
        return {"error": "start_failed"}
    try:
        streams = [bytearray(), bytearray()]
        truncated = [False, False]
        with selectors.DefaultSelector() as selector:
            selector.register(child.stdout, selectors.EVENT_READ, 0)
            selector.register(child.stderr, selectors.EVENT_READ, 1)
            while selector.get_map():
                for key, _ in selector.select():
                    block = key.fileobj.read1(65536)
                    if not block:
                        selector.unregister(key.fileobj)
                        key.fileobj.close()
                        continue
                    index = key.data
                    remaining = 1048576 - len(streams[index])
                    streams[index].extend(block[:remaining])
                    truncated[index] |= len(block) > remaining
        code = child.wait()
        return dict(exit_code=code, stdout=base64.b64encode(streams[0]).decode("ascii"),
                    stderr=base64.b64encode(streams[1]).decode("ascii"),
                    stdout_truncated=truncated[0], stderr_truncated=truncated[1])
    except Exception:
        return {"error": "execution_failed"}


try:
    response = json.dumps(run(), separators=(",", ":"), ensure_ascii=True).encode("ascii")
    if len(response) > 3145728:
        response = b'{"error":"execution_failed"}'
    sys.stdout.buffer.write(response)
    sys.stdout.buffer.flush()
except Exception:
    raise SystemExit(1)
