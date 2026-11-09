"""Test-only API interposition; executed over real SSH as runner."""
import os
import stat
import socket


def cases(source):
    ns = {"__name__": "test_receiver"}
    exec(source, ns)
    update = ns["update"]
    base = "/home/runner/file-content-cases"
    os.mkdir(base, 0o700)
    p = base + "/target"
    victim = base + "/victim"
    def put(path, data):
        with open(path, "wb") as f:
            f.write(data)
    def read(path):
        with open(path, "rb") as f:
            return f.read()
    def expect(value, wanted):
        assert value == wanted, (value, wanted)
    yes, no = {"changed": True}, {"changed": False}
    false, unknown = {"error": "write_failed"}, {"error": "execution_failed"}
    foreign = '/home/runner/foreign-file'
    before = os.stat(foreign)
    expect(update(foreign, b'replacement'), false)
    after = os.stat(foreign)
    assert read(foreign) == b'foreign'
    assert (before.st_uid, before.st_gid, before.st_mode, before.st_mtime_ns, before.st_ino) == (after.st_uid, after.st_gid, after.st_mode, after.st_mtime_ns, after.st_ino)
    assert not [x for x in os.listdir('/home/runner') if x.startswith('.wintable-')]
    print('PASS foreign-owner')
    import struct
    acl = struct.pack('<I', 2) + b''.join(struct.pack('<HHI', *x) for x in [(1,7,0xffffffff),(2,0,12345),(4,0,0xffffffff),(16,0,0xffffffff),(32,0,0xffffffff)])
    for sg in (0,1):
        for default in (0,1):
            directory = f'/home/runner/attrs-{sg}-{default}'
            if default:
                os.setxattr(directory, 'system.posix_acl_default', acl)
            old = os.umask(0o077)
            try:
                dest = directory + '/new'
                expect(update(dest, b'new'), false if default else yes)
                if default:
                    assert not os.path.exists(dest)
                    assert os.getxattr(directory, 'system.posix_acl_default') == acl
                else:
                    s = os.stat(dest)
                    assert read(dest) == b'new'
                    assert (s.st_uid, s.st_gid, stat.S_IMODE(s.st_mode)) == (os.geteuid(), os.getegid(), 0o600)
                assert not [x for x in os.listdir(directory) if x.startswith('.wintable-')]
                print(f'PASS setgid-{sg}-acl-{default}')
            finally:
                os.umask(old)
    for data in (b"", "日本語😀\r\n\0\n".encode(), b"x" * 1048576):
        expect(update(p, data), yes)
        s = os.stat(p)
        assert read(p) == data and stat.S_IMODE(s.st_mode) == 0o600
        # An audit hook observes actual temp open events, not just residue.
        events = []
        import sys
        sys.addaudithook(lambda event, args: events.append(args[0]) if event == "open" and isinstance(args[0], str) and args[0].startswith(".wintable-") else None)
        expect(update(p, data), no)
        assert os.stat(p).st_mtime_ns == s.st_mtime_ns and not events
    print("PASS bytes-equal")
    os.chmod(p, 0o6750)
    before = os.stat(p)
    expect(update(p, b"mode"), yes)
    after = os.stat(p)
    assert (before.st_uid, before.st_gid, stat.S_IMODE(before.st_mode)) == (after.st_uid, after.st_gid, stat.S_IMODE(after.st_mode))
    print("PASS mode-retention")
    put(victim, b"victim")
    os.unlink(p)
    os.symlink(victim, p)
    expect(update(p, b"bad"), false)
    os.unlink(p)
    os.mkdir(p)
    expect(update(p, b"bad"), false)
    os.rmdir(p)
    os.mkfifo(p)
    expect(update(p, b"bad"), false)
    os.unlink(p)
    sock = socket.socket(socket.AF_UNIX)
    sock.bind(p)
    expect(update(p, b"bad"), false)
    sock.close()
    os.unlink(p)
    os.symlink(base, base + "/link")
    expect(update(base + "/link/target", b"bad"), false)
    expect(update(base + "/missing/target", b"bad"), false)
    expect(update("/tmp/rejected-wintable", b"bad"), false)
    print("PASS type-ancestor-rejections")
    put(p, b"old")
    os.setxattr(p, "user.test", b"x")
    expect(update(p, b"old"), false)
    os.removexattr(p, "user.test")
    # ACL xattrs use Linux's documented v2 encoding: named user grants nothing.
    import struct
    acl = struct.pack("<I", 2) + b"".join(struct.pack("<HHI", *x) for x in [(1,6,0xffffffff),(2,0,12345),(4,0,0xffffffff),(16,0,0xffffffff),(32,0,0xffffffff)])
    os.setxattr(p, "system.posix_acl_access", acl)
    expect(update(p, b"bad"), false)
    os.removexattr(p, "system.posix_acl_access")
    os.chmod(base, 0o500)
    expect(update(p, b"bad"), false)
    os.chmod(base, 0o700)
    print("PASS acl-xattr-permissions")
    # Inject just before the real atomic API. Replacement must not follow link.
    replace = os.replace
    def swap(src, dst, **kw):
        os.unlink(dst, dir_fd=kw["dst_dir_fd"])
        os.symlink(victim, dst, dir_fd=kw["dst_dir_fd"])
        return replace(src, dst, **kw)
    os.replace = swap
    try:
        expect(update(p, b"whole"), yes)
        assert read(victim) == b"victim" and read(p) == b"whole" and not os.path.islink(p)
    finally:
        os.replace = replace
    print("PASS target-link-swap")
    # Destination appearing at new-file commit cannot be overwritten.
    os.unlink(p)
    link = os.link
    def appear(src, dst, **kw):
        put(p, b"concurrent")
        return link(src, dst, **kw)
    os.link = appear
    try:
        expect(update(p, b"new"), unknown)
        assert read(p) == b"concurrent"
    finally:
        os.link = link
    print("PASS new-destination-race")
    # Commit/sync/cleanup failures retain the conservative null classification.
    def fail(*a, **kw):
        raise OSError()
    os.replace = fail
    try:
        expect(update(p, b"new"), unknown)
    finally:
        os.replace = replace
    unlink = os.unlink
    os.unlink(p)
    os.unlink = fail
    try:
        expect(update(p, b"new"), unknown)
        assert read(p) == b"new"
    finally:
        os.unlink = unlink
    leftovers = [x for x in os.listdir(base) if x.startswith(".wintable-")]
    assert len(leftovers) == 1
    for x in leftovers:
        os.unlink(base + "/" + x)
    fsync = os.fsync
    os.fsync = fail
    try:
        expect(update(p, b"newer"), false)
    finally:
        os.fsync = fsync
    print("PASS commit-cleanup-fsync")
    # Ancestor rename before revalidation is refused; cleanup uses pinned dirfd.
    moved = base + "-moved"
    def move_at_sync(fd):
        if stat.S_ISREG(os.fstat(fd).st_mode):
            os.rename(base, moved)
            os.mkdir(base, 0o700)
        return fsync(fd)
    os.fsync = move_at_sync
    try:
        expect(update(p, b"moved"), false)
        assert read(moved + "/target") == b"new"
        assert os.listdir(base) == []
        assert not [x for x in os.listdir(moved) if x.startswith(".wintable-")]
    finally:
        os.fsync = fsync
        os.rmdir(base)
        os.rename(moved, base)
    print("PASS ancestor-rename")
    # A substituted temp must never be removed as our own cleanup.
    def substitute(fd):
        if stat.S_ISREG(os.fstat(fd).st_mode):
            tmp = next(x for x in os.listdir(base) if x.startswith(".wintable-"))
            os.rename(base + "/" + tmp, base + "/owned-temp")
            os.symlink(victim, base + "/" + tmp)
        return fsync(fd)
    os.fsync = substitute
    try:
        expect(update(p, b"substitute"), unknown)
        assert read(victim) == b"victim"
        assert read(p) == b"new"
        foreign = next(x for x in os.listdir(base) if x.startswith(".wintable-"))
        assert os.path.islink(base + "/" + foreign)
        os.unlink(base + "/" + foreign)
        os.unlink(base + "/owned-temp")
    finally:
        os.fsync = fsync
    assert not [x for x in os.listdir(base) if x.startswith(".wintable-")]
    print("PASS temp-substitution")
    print("file-content API cases passed")
