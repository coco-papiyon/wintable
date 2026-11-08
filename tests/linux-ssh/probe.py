"""Fixed synthetic transport probe, independent of the product receiver.

Only counts and digests are persisted. Mode names are fixed by the test.
"""
import codecs
import hashlib
import json
import os
import sys
import threading
import time

mode = sys.argv[1]
if mode not in ("hash", "flood", "wait", "no-read"):
    raise SystemExit(64)
with open("/fixture/results/starts", "a") as log:
    log.write(mode + "\n")
if mode == "no-read":
    time.sleep(120)
    raise SystemExit(0)
digest = hashlib.sha256()
decoder = codecs.getincrementaldecoder("utf-8")("strict")
length = 0
splits = 0
while True:
    block = os.read(0, 7)  # Force decoder boundaries, not TCP packet boundaries.
    if not block:
        break
    digest.update(block)
    length += len(block)
    decoder.decode(block)
    if decoder.getstate()[0]:
        splits += 1
decoder.decode(b"", final=True)
record = dict(length=length, sha256=digest.hexdigest(), eof=True,
              split_reads=splits, uid=os.geteuid())
with open("/fixture/results/" + mode + ".json", "w") as log:
    json.dump(record, log)
if mode == "hash":
    print(json.dumps(record))
elif mode == "flood":
    def write(fd, byte):
        data = byte * (1024 * 1024 + 65537)
        while data:
            data = data[os.write(fd, data):]
    other = threading.Thread(target=write, args=(2, b"E"))
    other.start()
    write(1, b"O")
    other.join()
    raise SystemExit(23)
elif mode == "wait":
    time.sleep(120)
