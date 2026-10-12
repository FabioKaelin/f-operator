"""Bounded readiness polling, independent of proxy buffering of Kubernetes watches."""
import json
import subprocess
import sys
import time
kind,name=sys.argv[1].split('/',1);namespace=sys.argv[2]
deadline=time.monotonic()+300
while time.monotonic()<deadline:
    try:
        raw=subprocess.check_output(['kubectl','--request-timeout=15s','get',kind,name,'-n',namespace,'-o','json'],text=True,timeout=25)
        obj=json.loads(raw);spec=obj['spec'];status=obj.get('status',{});wanted=spec.get('replicas',1)
        fresh=status.get('observedGeneration',0)>=obj['metadata']['generation']
        ready=status.get('readyReplicas',0)>=wanted and status.get('updatedReplicas',0)>=wanted
        complete=status.get('replicas',0)==wanted and status.get('terminatingReplicas',0)==0
        if fresh and ready and complete:
            print('Ready:',namespace,kind,name);break
    except (subprocess.SubprocessError,ValueError):pass
    time.sleep(2)
else:raise SystemExit('Readiness deadline exceeded: '+namespace+'/'+name)
