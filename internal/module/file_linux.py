"""Directory receiver. Descriptor-relative operations never follow links."""
import json
import os
import sys
import unicodedata

def obj(pairs):
    d = {}
    for k, v in pairs:
        if k in d:
            raise ValueError()
        d[k] = v
    return d

r = {'error': 'invalid_request'}
fds = []
try:
    data = sys.stdin.buffer.read(2097153)
    if len(data) > 2097152:
        raise ValueError()
    d = json.loads(data.decode('utf-8'), object_pairs_hook=obj)
    if not isinstance(d, dict) or set(d) != {'module', 'args'} or d['module'] != 'file':
        raise ValueError()
    a = d['args']
    if not isinstance(a, dict) or set(a) != {'path', 'state'} or a['state'] != 'directory':
        raise ValueError()
    p = a['path']
    if not isinstance(p, str) or not p.startswith('/') or any(unicodedata.category(c) == 'Cc' for c in p):
        raise ValueError()
    p.encode('utf-8')
    parts = p[1:].split('/')
    if any(c in ('', '.', '..') for c in parts):
        raise ValueError()
    r = {'error': 'execution_failed'}
    flags = os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW | os.O_CLOEXEC
    fd = os.open('/', flags)
    fds.append(fd)
    changed = False
    for c in parts:
        try:
            child = os.open(c, flags, dir_fd=fd)
        except FileNotFoundError:
            try:
                os.mkdir(c, dir_fd=fd)
                changed = True
            except FileExistsError:
                pass
            child = os.open(c, flags, dir_fd=fd)
        fds.append(child)
        fd = child
    r = {'changed': changed}
except Exception:
    pass
finally:
    for fd in reversed(fds):
        os.close(fd)
sys.stdout.write(json.dumps(r, separators=(',', ':')))
