"""Apply repository policy without changing replicas, images or persistence."""
import json
import subprocess
import time
import sys
from pathlib import Path
policy=json.loads(Path(__file__).with_name('policy.json').read_text())
def call(*args):return subprocess.check_output(['kubectl','--request-timeout=20s',*args],text=True)
for group in policy['groups']:
    priority='fabkli-'+group['name']
    for kind,ns,name in group['apps']+group.get('databases',[]):
        obj=json.loads(call('get',kind,name,'-n',ns,'-o','json'))
        if kind=='fdeployments':patch={'spec':{'priorityClassName':priority}}
        else:patch={'spec':{'template':{'spec':{'priorityClassName':priority,'terminationGracePeriodSeconds':300 if kind=='statefulsets' else 120}}}}
        before=obj['spec'].get('priorityClassName') if kind=='fdeployments' else obj['spec']['template']['spec'].get('priorityClassName')
        call('patch',kind,name,'-n',ns,'--type=merge','-p',json.dumps(patch))
        if before!=priority:
            workload='deployment' if kind=='fdeployments' else kind
            for _ in range(60):
                observed=json.loads(call('get',workload,name,'-n',ns,'-o','json'))
                if observed['spec']['template']['spec'].get('priorityClassName')==priority:break
                time.sleep(2)
            else:raise RuntimeError('Operator priority was not reconciled')
            subprocess.run([sys.executable,str(Path(__file__).with_name('wait-ready.py')),workload+'/'+name,ns],check=True)
# Monitoring operator regenerates StatefulSets; persist priority in its owning CRs.
for kind in ['prometheus','alertmanager']:
    for resource in json.loads(call('get',kind,'-n','monitoring','-o','json'))['items']:
        call('patch',kind,resource['metadata']['name'],'-n','monitoring','--type=merge','-p',json.dumps({'spec':{'priorityClassName':'fabkli-infrastructure'}}))
# Leave Kubernetes system-critical priority classes intact. Protect supporting deployments.
for ns in ['envoy-gateway-system','f-operator-system','monitoring','kube-system']:
    for obj in json.loads(call('get','deployments,statefulsets,daemonsets','-n',ns,'-o','json'))['items']:
        name=obj['metadata']['name']
        if name=='grafana':continue
        kind=obj['kind'].lower();current=obj['spec']['template']['spec'].get('priorityClassName','')
        if current.startswith('system-'):continue
        call('patch',kind,name,'-n',ns,'--type=merge','-p',json.dumps({'spec':{'template':{'spec':{'priorityClassName':'fabkli-infrastructure'}}}}))
        subprocess.run([sys.executable,str(Path(__file__).with_name('wait-ready.py')),kind+'/'+name,ns],check=True)
call('patch','deployment','f-gateway','-n','arr-stack','--type=merge','-p',json.dumps({'spec':{'template':{'spec':{'priorityClassName':'fabkli-infrastructure'}}}}))
print('Owner workload priorities applied; system-critical priorities preserved.')
