"""Fixed receiver; trusted root/SSH-user namespace only. No task shell code."""
import json
import os
import platform
import secrets
import stat
import sys
import unicodedata


def reject():
    raise ValueError()


def obj(pairs):
    result = {}
    for k, v in pairs:
        if k in result:
            reject()
        result[k] = v
    return result


def attributes(fd):
    s = os.fstat(fd)
    if os.listxattr(fd):
        reject()
    return s


def directory(fd):
    s = attributes(fd)
    if not stat.S_ISDIR(s.st_mode) or s.st_uid not in (0, os.geteuid()) or s.st_mode & 0o022:
        reject()


def target(parent, name):
    try:
        fd = os.open(name, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK | os.O_CLOEXEC, dir_fd=parent)
    except FileNotFoundError:
        return None
    try:
        s = attributes(fd)
        if not stat.S_ISREG(s.st_mode) or s.st_nlink != 1:
            reject()
        return fd
    except Exception:
        os.close(fd)
        raise


def update(path, data):
    fds = []
    temp = None
    tempfd = None
    started = False
    response = {"error": "write_failed"}
    try:
        parent = os.open("/", os.O_RDONLY | os.O_DIRECTORY | os.O_CLOEXEC)
        fds.append(parent)
        directory(parent)
        parts = path[1:].split("/")
        for part in parts[:-1]:
            parent = os.open(part, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW | os.O_CLOEXEC, dir_fd=parent)
            fds.append(parent)
            directory(parent)
        name = parts[-1]
        old = target(parent, name)
        before = None
        if old is not None:
            fds.append(old)
            before = attributes(old)
            # Bound reads even when the existing file is larger than the limit.
            blocks = bytearray()
            while len(blocks) <= len(data):
                b = os.read(old, min(65536, len(data) + 1 - len(blocks)))
                if not b:
                    break
                blocks.extend(b)
            if blocks == data:
                # Close before returning so close failures remain unknown.
                for fd in reversed(fds):
                    os.close(fd)
                fds.clear()
                return {"changed": False}
        # Prove directory sync support before creating a temp.
        os.fsync(parent)
        temp = ".wintable-" + secrets.token_hex(16)
        tempfd = os.open(temp, os.O_RDWR | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW | os.O_CLOEXEC, 0o600, dir_fd=parent)
        fds.append(tempfd)
        view = memoryview(data)
        while view:
            n = os.write(tempfd, view)
            if n <= 0:
                reject()
            view = view[n:]
        uid, gid, mode = (before.st_uid, before.st_gid, stat.S_IMODE(before.st_mode)) if before else (os.geteuid(), os.getegid(), 0o600)
        os.fchown(tempfd, uid, gid)
        os.fchmod(tempfd, mode)
        s = attributes(tempfd)
        if (s.st_uid, s.st_gid, stat.S_IMODE(s.st_mode)) != (uid, gid, mode):
            reject()
        os.fsync(tempfd)
        for fd in fds[:len(parts)]:
            directory(fd)
        for i, part in enumerate(parts[:-1]):
            named = os.stat(part, dir_fd=fds[i], follow_symlinks=False)
            opened = os.fstat(fds[i + 1])
            if (named.st_dev, named.st_ino) != (opened.st_dev, opened.st_ino):
                reject()
        check = target(parent, name)
        if check is not None:
            os.close(check)
        if os.stat(temp, dir_fd=parent, follow_symlinks=False).st_ino != s.st_ino:
            reject()
        started = True
        if before is not None:
            os.replace(temp, name, src_dir_fd=parent, dst_dir_fd=parent)
            temp = None
        else:
            os.link(temp, name, src_dir_fd=parent, dst_dir_fd=parent, follow_symlinks=False)
        os.fsync(parent)
        response = {"changed": True}
    except Exception:
        response = {"error": "execution_failed" if started else "write_failed"}
    finally:
        if temp is not None and tempfd is not None:
            try:
                s = os.stat(temp, dir_fd=parent, follow_symlinks=False)
                own = os.fstat(tempfd)
                if (s.st_dev, s.st_ino) != (own.st_dev, own.st_ino):
                    reject()
                os.unlink(temp, dir_fd=parent)
                os.fsync(parent)
            except Exception:
                response = {"error": "execution_failed"}
        for fd in reversed(fds):
            try:
                os.close(fd)
            except Exception:
                response = {"error": "execution_failed"}
    return response


def run():
    try:
        if sys.platform != "linux" or platform.python_implementation() != "CPython" or sys.version_info[:2] != (3, 13):
            reject()
        raw = sys.stdin.buffer.read(2097153)
        if len(raw) > 2097152:
            reject()
        r = json.loads(raw.decode("utf-8", "strict"), object_pairs_hook=obj, parse_constant=lambda _: reject())
        if not isinstance(r, dict) or set(r) != {"module", "args"} or r["module"] != "file_content":
            reject()
        a = r["args"]
        if not isinstance(a, dict) or set(a) != {"path", "content"}:
            reject()
        p, c = a["path"], a["content"]
        if not isinstance(p, str) or not isinstance(c, str):
            reject()
        p.encode("utf-8", "strict")
        data = c.encode("utf-8", "strict")
        if not p.startswith("/") or any(x in ("", ".", "..") for x in p[1:].split("/")) or any(unicodedata.category(x) == "Cc" for x in p) or len(data) > 1048576:
            reject()
    except Exception:
        return {"error": "invalid_request"}
    return update(p, data)


if __name__ == "__main__":
    try:
        sys.stdout.write(json.dumps(run(), separators=(",", ":")))
        sys.stdout.flush()
    except Exception:
        raise SystemExit(1)
