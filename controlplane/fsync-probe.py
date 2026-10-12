"""Small sequential durable-write probe, temporary file only; reject excessive p99."""
import os,sys,tempfile,time,json,statistics
fd,name=tempfile.mkstemp(prefix='.etcd-fsync-',dir=sys.argv[1]);samples=[]
try:
    for _ in range(128):
        start=time.monotonic();os.write(fd,b'\0'*2300);os.fdatasync(fd)
        samples.append((time.monotonic()-start)*1000)
finally:os.close(fd);os.unlink(name)
samples.sort();p99=samples[min(len(samples)-1,int(len(samples)*.99))]
print(json.dumps({'writes':len(samples),'median_ms':round(statistics.median(samples),3),'p99_ms':round(p99,3),'max_ms':round(samples[-1],3)}))
if p99>10:raise SystemExit('SSD sync latency exceeds maintenance preflight bound')
