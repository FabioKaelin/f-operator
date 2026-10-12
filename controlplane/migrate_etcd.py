"""Offline, same-member storage move. Keep SD source and original manifest for recovery."""
import datetime,json,os,pathlib,subprocess,hashlib
root=pathlib.Path('/var/lib/minikube/controlplane')
old=pathlib.Path('/var/lib/minikube/etcd');new=pathlib.Path('/var/lib/minikube/etcd-ssd/data')
manifest=pathlib.Path('/etc/kubernetes/manifests/etcd.yaml')

def run(*args):return subprocess.check_output(args,text=True,stderr=subprocess.STDOUT).strip()
def journal(phase):
    p=root/'migration.json.tmp';p.write_text(json.dumps({'phase':phase,'at':datetime.datetime.now(datetime.timezone.utc).isoformat(),'source':str(old),'target':str(new)}));p.chmod(0o600);p.replace(root/'migration.json')
def hashes(folder):
    result={}
    for path in sorted(folder.rglob('*')):
        if path.is_symlink():raise ValueError('Unexpected symlink in etcd member')
        if path.is_file():
            h=hashlib.sha256()
            with path.open('rb') as f:
                for chunk in iter(lambda:f.read(1024*1024),b''):h.update(chunk)
            result[str(path.relative_to(folder))]=h.hexdigest()
    return result

def main():
    if os.geteuid()!=0:raise ValueError('Root required')
    run(str(root/'mount-etcd-ssd.sh'))
    contents=manifest.read_text()
    oldpath='      path: /var/lib/minikube/etcd\n'
    newpath='      path: /var/lib/minikube/etcd-ssd/data\n'
    if newpath in contents:
        if not (new/'member/snap/db').is_file():raise ValueError('Missing migrated member')
        print('etcd SSD hostPath already configured; no restart needed');return
    if contents.count(oldpath)!=1:raise ValueError('Unexpected etcd manifest')
    if new.exists():raise ValueError('Target data already exists; refusing overwrite')
    if not (old/'member/snap/db').is_file() or not (old/'pre-ssd-snapshot.db').is_file():raise ValueError('Source or required snapshot missing')
    original=root/'etcd.original.yaml'
    if original.exists():raise ValueError('Earlier maintenance exists; review first')
    original.write_text(contents);original.chmod(0o600)
    journal('preparing')
    stopped=False
    try:
        run('systemctl','stop','kubelet');stopped=True
        cri=['crictl','--runtime-endpoint','unix:///var/run/cri-dockerd.sock']
        for container in run(*cri,'ps','--name','etcd','-q').splitlines():
            run(*cri,'stop','--timeout','60',container)
        if run(*cri,'ps','--name','etcd','-q'):raise ValueError('etcd is still running')
        journal('etcd-stopped')
        pending=new.with_name('data.pending')
        if pending.exists():raise ValueError('Earlier pending data exists; refusing overwrite')
        pending.mkdir(mode=0o700)
        run('cp','-a',str(old/'member'),str(pending/'member'))
        if hashes(old/'member')!=hashes(pending/'member'):raise ValueError('Offline member checksum mismatch')
        run('sync','-f',str(pending));pending.rename(new)
        journal('data-copied')
        run('install','-m','644',str(root/'etcd-ssd.service'),'/etc/systemd/system/etcd-ssd.service')
        run('install','-d','/etc/systemd/system/kubelet.service.d')
        run('install','-m','644',str(root/'kubelet-etcd-ssd.conf'),'/etc/systemd/system/kubelet.service.d/30-etcd-ssd.conf')
        run('systemctl','daemon-reload');run('systemctl','enable','etcd-ssd.service')
        temp=manifest.with_name('.etcd.ssd.tmp');temp.write_text(contents.replace(oldpath,newpath));temp.chmod(0o600);temp.replace(manifest)
        journal('manifest-configured')
    finally:
        if stopped:run('systemctl','start','kubelet')
    journal('kubelet-started')
    print('Same-member etcd data migrated; original SD data and manifest retained. Verify endpoint/API health.')
if __name__=='__main__':main()
