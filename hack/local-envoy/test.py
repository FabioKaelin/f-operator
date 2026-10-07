"""Real standalone Gateway API translation/proxy test. No kubeconfig, kubectl or cluster."""
import json, os, pathlib, shutil, subprocess, tempfile, time, urllib.request, uuid
ROOT=pathlib.Path(__file__).resolve().parent
network='foperator-test-'+uuid.uuid4().hex[:10]
names=[]
user=str(os.getuid())+':'+str(os.getgid())
def run(*args,**kwargs):return subprocess.run(args,check=True,text=True,**kwargs)
try:
    with tempfile.TemporaryDirectory(prefix='foperator-envoy-') as work:
        root=pathlib.Path(work)
        root.chmod(0o777)
        (root/'config').mkdir(mode=0o777)
        shutil.copy(ROOT/'standalone.yaml',root/'standalone.yaml')
        shutil.copy(ROOT/'routes.yaml',root/'config'/'routes.yaml')
        run('docker','network','create',network,stdout=subprocess.DEVNULL)
        run('docker','run','--rm','--workdir','/tmp/envoy-gateway','--user',user,'--env','USER=foperator-test','--env','HOME=/tmp/envoy-gateway','--memory','128m','--memory-swap','128m','--cpus','0.5','--volume',str(root)+':/tmp/envoy-gateway','envoyproxy/gateway:v1.9.2','certgen','--local','--config-home','/tmp/envoy-gateway')
        for kind in ['backend','frontend']:
            name=network+'-'+kind;names.append(name)
            run('docker','run','--detach','--name',name,'--network',network,'--network-alias','dummy-'+kind,'--memory','64m','--memory-swap','64m','--cpus','0.5','--volume',str(ROOT/'backend.py')+':/backend.py:ro','--env','BACKEND_NAME='+kind,'python:3.14.0-slim','python','/backend.py',stdout=subprocess.DEVNULL)
        name=network+'-gateway';names.append(name)
        run('docker','run','--detach','--name',name,'--network',network,'--workdir','/tmp/envoy-gateway','--user',user,'--env','USER=foperator-test','--env','HOME=/tmp/envoy-gateway','--publish','127.0.0.1::8888','--memory','512m','--memory-swap','512m','--cpus','1','--volume',str(root)+':/tmp/envoy-gateway','envoyproxy/gateway:v1.9.2','server','--config-path','/tmp/envoy-gateway/standalone.yaml',stdout=subprocess.DEVNULL)
        address=run('docker','port',name,'8888/tcp',capture_output=True).stdout.strip()
        def request(path,host='dummy.example'):
            req=urllib.request.Request('http://'+address+path,headers={'Host':host,'X-Forwarded-Proto':'https','X-Forwarded-Host':host,'X-Forwarded-Port':'443','X-Forwarded-For':'192.0.2.9','X-Real-IP':'192.0.2.9'})
            with urllib.request.urlopen(req,timeout=3) as response:return json.load(response)
        deadline=time.monotonic()+180
        while True:
            try:
                request('/api');break
            except Exception:
                if time.monotonic()>=deadline:
                    run('docker','logs',name);raise
                time.sleep(1)
        for path,expected in [('/api','backend'),('/api/child?q=one','backend'),('/apix','frontend'),('/','frontend')]:
            data=request(path)
            assert data['backend']==expected,data
            assert data['path']==path and data['host']=='dummy.example',data
            assert data['proto']=='https' and data['forwarded_host']=='dummy.example' and data['forwarded_port']=='443' and data['real_ip']=='192.0.2.9',data
        try:
            request('/','unknown.example');raise AssertionError('unexpected host accepted')
        except urllib.error.HTTPError as error:assert error.code==404
        # File-provider reconciliation: change the backend route and wait for xDS.
        routes=(root/'config'/'routes.yaml').read_text().replace('value: /api','value: /v2')
        (root/'config'/'routes.yaml').write_text(routes)
        deadline=time.monotonic()+30
        while request('/v2')['backend']!='backend':
            assert time.monotonic()<deadline,'route update timed out'
            time.sleep(.5)
        assert request('/api')['backend']=='frontend'
        print('PASS: real Envoy Gateway prefix precedence, host isolation, query preservation, forwarded HTTPS headers, and xDS route update')
finally:
    for name in reversed(names):subprocess.run(['docker','rm','--force',name],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
    subprocess.run(['docker','network','rm',network],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
