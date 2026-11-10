"""Fixed isolated Linux shell receiver. Task values arrive only on stdin."""
import base64
import json
import platform
import selectors
import subprocess
import sys
import threading


def reject():
    raise ValueError()


def obj(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            reject()
        result[key] = value
    return result


def run():
    try:
        if sys.platform != "linux" or platform.python_implementation() != "CPython" or sys.version_info[:2] != (3, 13):
            reject()
        raw = sys.stdin.buffer.read(2097153)
        if len(raw) > 2097152:
            reject()
        request = json.loads(raw.decode("utf-8", "strict"), object_pairs_hook=obj,
                             parse_constant=lambda _: reject())
        if not isinstance(request, dict) or set(request) != {"module", "args"} or request["module"] != "shell":
            reject()
        args = request["args"]
        if not isinstance(args, dict) or set(args) != {"script"}:
            reject()
        script = args["script"]
        if not isinstance(script, str) or not script.strip() or "\0" in script:
            reject()
        code = script.encode("utf-8", "strict")
        if len(code) > 1048576:
            reject()
    except Exception:
        return {"error": "invalid_request"}
    try:
        child = subprocess.Popen(["/bin/sh"], shell=False, stdin=subprocess.PIPE,
                                 stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    except Exception:
        return {"error": "start_failed"}
    try:
        write_failed = []
        def feed():
            try:
                child.stdin.write(code)
                child.stdin.close()
            except BrokenPipeError:
                pass
            except Exception:
                write_failed.append(True)
            finally:
                try:
                    child.stdin.close()
                except BrokenPipeError:
                    pass
                except Exception:
                    write_failed.append(True)
        writer = threading.Thread(target=feed)
        writer.start()
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
        status = child.wait()
        writer.join()
        if write_failed:
            raise ValueError()
        return dict(exit_code=status, stdout=base64.b64encode(streams[0]).decode("ascii"),
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
