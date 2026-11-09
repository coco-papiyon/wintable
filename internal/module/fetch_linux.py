import os,sys,json,stat,hashlib,struct,platform
def pairs(items):
 d={}
 for k,v in items:
  if k in d: raise ValueError()
  d[k]=v
 return d
def run():
 if sys.platform!='linux' or platform.python_implementation()!='CPython' or sys.version_info[:2]!=(3,13): raise ValueError()
 raw=sys.stdin.buffer.read(2097153)
 if len(raw)>2097152: raise ValueError()
 d=json.loads(raw.decode('utf-8'),object_pairs_hook=pairs)
 if set(d)!={'module','args'} or d['module']!='fetch' or set(d['args'])!={'src'}: raise ValueError()
 p=d['args']['src']
 if not isinstance(p,str) or not p.startswith('/') or len(p)>4096 or any(ord(c)<32 or 127<=ord(c)<=159 or c in '*?[' for c in p): raise ValueError()
 parts=p[1:].split('/')
 if any(c in ('','.','..') for c in parts): raise ValueError()
 p.encode('utf-8',errors='strict')
 guards=[os.open('/',os.O_RDONLY|os.O_DIRECTORY|os.O_NOFOLLOW)]
 try:
  for c in parts[:-1]: guards.append(os.open(c,os.O_RDONLY|os.O_DIRECTORY|os.O_NOFOLLOW,dir_fd=guards[-1]))
  fd=os.open(parts[-1],os.O_RDONLY|os.O_NOFOLLOW|os.O_NONBLOCK,dir_fd=guards[-1])
  with os.fdopen(fd,'rb',buffering=0) as f:
   first=os.fstat(f.fileno())
   if not stat.S_ISREG(first.st_mode) or first.st_nlink!=1 or first.st_size>1073741824: raise ValueError()
   def stamp(s): return (s.st_dev,s.st_ino,s.st_size,s.st_mtime_ns,s.st_ctime_ns,s.st_nlink)
   def digest():
    f.seek(0);h=hashlib.sha256();n=0
    while True:
     b=f.read(65536)
     if not b: break
     n+=len(b)
     if n>first.st_size: raise ValueError()
     h.update(b)
    if n!=first.st_size: raise ValueError()
    return h.digest()
   before=digest();f.seek(0);h=hashlib.sha256();n=0;out=sys.stdout.buffer
   out.write(b'WTF1'+struct.pack('<Q',first.st_size))
   while True:
    b=f.read(65536)
    if not b: break
    n+=len(b)
    if n>first.st_size: raise ValueError()
    h.update(b);out.write(b)
   if n!=first.st_size or before!=h.digest() or before!=digest() or stamp(first)!=stamp(os.fstat(f.fileno())): raise ValueError()
   out.write(h.digest());out.flush()
 finally:
  for fd in reversed(guards): os.close(fd)
try:
 run()
except BaseException:
 os._exit(1)
