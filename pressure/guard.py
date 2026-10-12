"""Graceful ordered pressure shedding. Never delete pods, volumes or database files."""
import datetime as dt
import json
import os
import ssl
import time
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path
CONFIG=json.loads(Path(os.environ.get('PRESSURE_CONFIG','/config/policy.json')).read_text()) if __name__=='__main__' else {}
NS='f-operator-system'
STATE_NAME='pressure-state'
class API:
    def __init__(self):
        directory=Path('/var/run/secrets/kubernetes.io/serviceaccount')
        self.token_path=directory/'token'
        self.base='https://'+os.environ['KUBERNETES_SERVICE_HOST']+':'+os.environ.get('KUBERNETES_SERVICE_PORT_HTTPS','443')
        self.opener=urllib.request.build_opener(urllib.request.ProxyHandler({}),urllib.request.HTTPSHandler(context=ssl.create_default_context(cafile=str(directory/'ca.crt'))))
    def call(self,path,method='GET',body=None):
        headers={'Authorization':'Bearer '+self.token_path.read_text().strip(),'Content-Type':'application/merge-patch+json' if method=='PATCH' else 'application/json'}
        request=urllib.request.Request(self.base+path,headers=headers,method=method,data=json.dumps(body).encode() if body is not None else None)
        with self.opener.open(request,timeout=8) as response:
            data=response.read(4*1024*1024+1)
            if len(data)>4*1024*1024:raise ValueError('API response too large')
            return json.loads(data)

def path(target):
    kind,ns,name=target
    prefix='/apis/k8s.fabkli.ch/v1' if kind=='fdeployments' else '/apis/apps/v1'
    return prefix+'/namespaces/'+ns+'/'+kind+'/'+name

def selector(obj): return ','.join(k+'='+v for k,v in obj['spec']['selector']['matchLabels'].items())

def pods(api,target):
    obj=api.call(path(target))
    if target[0]=='fdeployments':obj=api.call(path(['deployments',target[1],target[2]]))
    return api.call('/api/v1/namespaces/'+target[1]+'/pods?'+urllib.parse.urlencode({'labelSelector':selector(obj)}))['items']

class Guard:
    def __init__(self,api,policy):self.api=api;self.policy=policy
    def load(self):
        obj=self.api.call('/api/v1/namespaces/'+NS+'/configmaps/'+STATE_NAME)
        state=json.loads(obj['data']['state.json']);return obj,state
    def save(self,obj,state):
        result=self.api.call('/api/v1/namespaces/'+NS+'/configmaps/'+STATE_NAME,'PATCH',{'metadata':{'resourceVersion':obj['metadata']['resourceVersion']},'data':{'state.json':json.dumps(state)}})
        obj.update(result)
    def mutate(self,target,entry,stop):
        obj=self.api.call(path(target))
        if 'uid' in entry and obj['metadata'].get('uid') != entry['uid']:raise ValueError('Workload was recreated; manual review required')
        spec=obj['spec'];field='suspend' if target[0]=='fdeployments' else 'replicas'
        value=True if field=='suspend' else 0
        original=entry['original']
        annotation='fabkli.ch/pressure-owner'
        annotations=obj['metadata'].get('annotations',{})
        expected=original if stop else value
        if annotations.get(annotation) not in (None,entry['owner']):raise ValueError('Another pressure owner')
        if not stop and annotations.get(annotation)!=entry['owner']:
            # A pending journal entry may represent a mutation that never reached the API.
            if spec.get(field,False if field=='suspend' else 1)==original:return
            raise ValueError('Pressure ownership changed; manual review required')
        if spec.get(field,False if field=='suspend' else 1) not in (expected,value if stop else original):raise ValueError('Desired state changed; manual review required')
        self.api.call(path(target),'PATCH',{'metadata':{'resourceVersion':obj['metadata']['resourceVersion'],'annotations':{annotation:entry['owner'] if stop else None}},'spec':{field:value if stop else original}})
    def capture(self,target,owner):
        obj=self.api.call(path(target));field='suspend' if target[0]=='fdeployments' else 'replicas';value=obj['spec'].get(field,False if field=='suspend' else 1)
        return {'target':target,'original':value,'uid':obj['metadata'].get('uid'),'owner':owner,'done':False}
    def suspend(self,obj,state,group):
        active=state.get('active')
        if not active:
            owner=str(time.time_ns())
            entries=[self.capture(target,owner) for target in group['apps']+group.get('databases',[])]
            # Keep independently disabled resources disabled after recovery.
            entries=[e for e in entries if (not e['original'] if e['target'][0]=='fdeployments' else e['original']>0)]
            if not entries:return False
            active={'name':group['name'],'entries':entries,'app_count':sum(e['target'] in group['apps'] for e in entries),'mode':'stopping'}
            state['active']=active;self.save(obj,state) # Journal BEFORE any workload mutation.
        for index,entry in enumerate(active['entries']):
            if not entry['done']:
                if index>=active['app_count']:
                    if any(pods(self.api,target) for target in group['apps']):return False
                self.mutate(entry['target'],entry,True);entry['done']=True;self.save(obj,state)
            # Never start the next group while application/database pods are still terminating.
            if pods(self.api,entry['target']):return False
        state.setdefault('stopped',[]).append(active);state.pop('active',None);state['last_action']=time.time();self.save(obj,state)
        print(json.dumps({'event':'pressure_suspended','group':active['name']}),flush=True);return True
    def resume(self,obj,state):
        if not state.get('active'):
            if not state.get('stopped'):return False
            active=state['stopped'].pop();active['mode']='starting'
            for entry in active['entries']:entry['done']=False
            state['active']=active;self.save(obj,state)
        active=state['active'];entries=active['entries'];count=active['app_count']
        # Databases first; wait for readiness before restoring any app.
        for entry in entries[count:]+entries[:count]:
            if not entry['done']:
                self.mutate(entry['target'],entry,False);entry['done']=True;self.save(obj,state)
            objwork=self.api.call(path(entry['target']))
            if entry['target'][0]=='fdeployments':objwork=self.api.call(path(['deployments',entry['target'][1],entry['target'][2]]))
            expected=objwork['spec'].get('replicas',1)
            available=objwork.get('status',{}).get('readyReplicas',0)
            if available<expected or objwork.get('status',{}).get('observedGeneration',0)<objwork['metadata'].get('generation',1):return False
        state.pop('active',None);state['last_action']=time.time();state['good']=0;self.save(obj,state)
        print(json.dumps({'event':'pressure_restored','group':active['name']}),flush=True);return True
    def tick(self,available,cgroup_headroom,cpu,now=None):
        now=now or time.time();obj,state=self.load()
        low=available<self.policy['low_memory_mib'] or cgroup_headroom<self.policy['low_cgroup_mib']
        busy=cpu>95
        state['bad']=state.get('bad',0)+1 if low else 0
        state['busy']=state.get('busy',0)+1 if busy else 0
        healthy=available>self.policy['recover_memory_mib'] and cgroup_headroom>self.policy['recover_cgroup_mib'] and cpu<80
        state['good']=state.get('good',0)+1 if healthy else 0
        state['sample']={'at':dt.datetime.now(dt.timezone.utc).isoformat(),'available_mib':round(available),'cgroup_headroom_mib':round(cgroup_headroom),'cpu_percent':round(cpu,1)}
        self.save(obj,state)
        active=state.get('active')
        if active:
            if active['mode']=='stopping':self.suspend(obj,state,next(g for g in self.policy['groups'] if g['name']==active['name']))
            elif healthy:self.resume(obj,state)
            return
        cooldown=now-state.get('last_action',0)>=self.policy['cooldown_seconds']
        if cooldown and (state['bad']>=3 or state['busy']>=15):
            stopped={g['name'] for g in state.get('stopped',[])}
            for group in self.policy['groups']:
                if group['name'] not in stopped:
                    if self.suspend(obj,state,group) or state.get('active'):break
        elif cooldown and state['good']>=30:self.resume(obj,state)

def sample_cpu():
    return [int(x) for x in Path('/host-proc/stat').read_text().splitlines()[0].split()[1:9]]
def cgroup_headroom(maximum,current,stats):
    if maximum == 'max':return 1000000000
    # Inactive clean file cache is reclaimable; anonymous memory, shmem and
    # dirty/writeback pages remain charged. Clamp against inconsistent samples.
    reclaimable=max(0,stats.get('inactive_file',0)-stats.get('file_dirty',0)-stats.get('file_writeback',0))
    reclaimable=min(current,reclaimable)
    return (int(maximum)-current+reclaimable)/1024**2

def memory():
    values={line.split(':')[0]:int(line.split()[1]) for line in Path('/host-proc/meminfo').read_text().splitlines()}
    available=values['MemAvailable']/1024
    # Host and Minikube container headroom are distinct: whichever reaches pressure first matters.
    folder=Path('/host-cgroup')/MINIKUBE_CGROUP.lstrip('/')
    maximum=(folder/'memory.max').read_text().strip();current=int((folder/'memory.current').read_text())
    stats={line.split()[0]:int(line.split()[1]) for line in (folder/'memory.stat').read_text().splitlines()}
    headroom=cgroup_headroom(maximum,current,stats)
    return available,headroom

def discover_cgroup():
    import re
    for process in Path('/host-proc').glob('[0-9]*'):
        try:
            command=(process/'comm').read_text().strip()
            if command != 'kubelet': continue
            contents=(process/'cgroup').read_text()
            match=re.search(r'(/[^\n]*?docker-[a-f0-9]{64}\.scope)',contents)
            if match:return match[1]
        except OSError:continue
    raise ValueError('Cannot identify Minikube parent cgroup')

def main():
    global MINIKUBE_CGROUP
    MINIKUBE_CGROUP=os.environ.get('MINIKUBE_CGROUP','/')
    api=API();guard=Guard(api,CONFIG);last=sample_cpu()
    while True:
        time.sleep(20)
        try:
            current=sample_cpu();delta=[c-p for c,p in zip(current,last)];last=current
            total=sum(delta);cpu=100*(total-delta[3]-delta[4])/total if total>0 else 0
            guard.tick(*memory(),cpu)
        except Exception as e:
            # No raw URLs, tokens, bodies or private data in logs. API errors never force deletion.
            print(json.dumps({'event':'pressure_guard_error','type':type(e).__name__}),flush=True)
if __name__=='__main__':main()
