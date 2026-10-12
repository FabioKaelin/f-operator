"""Single privileged host maintenance job; no API token or externally supplied shell text."""
import json,sys
name='etcd-ssd-'+sys.argv[1]
if not sys.argv[1].isdigit():raise ValueError('Numeric GitHub run required')
command='''set -eu
mkdir -p /host/var/lib/minikube/controlplane
chmod 700 /host/var/lib/minikube/controlplane
cp /source/* /host/var/lib/minikube/controlplane/
chmod 700 /host/var/lib/minikube/controlplane/*.sh
nsenter -t 1 -m -u -i -n -p -r -- /bin/bash -c '
set -euo pipefail
if ! command -v cryptsetup >/dev/null; then
 apt-get update -qq
 DEBIAN_FRONTEND=noninteractive apt-get install -y -qq cryptsetup-bin
fi
/var/lib/minikube/controlplane/prepare-etcd-ssd.sh
python3 /var/lib/minikube/controlplane/migrate_etcd.py
'
'''
job={'apiVersion':'batch/v1','kind':'Job','metadata':{'name':name,'namespace':'f-operator-system'},'spec':{'backoffLimit':0,'activeDeadlineSeconds':600,'ttlSecondsAfterFinished':86400,'template':{'spec':{'restartPolicy':'Never','nodeName':'minikube','hostPID':True,'hostNetwork':True,'automountServiceAccountToken':False,'priorityClassName':'system-node-critical','containers':[{'name':'maintenance','image':'busybox@sha256:bdf57e528e45e4433820e045b29b4597825a1c9e38353532d90a01445013f82e','command':['/bin/sh','-c',command],'securityContext':{'privileged':True,'runAsUser':0},'resources':{'requests':{'cpu':'50m','memory':'64Mi'},'limits':{'cpu':'500m','memory':'256Mi'}},'volumeMounts':[{'name':'host','mountPath':'/host'},{'name':'source','mountPath':'/source','readOnly':True}]}],'volumes':[{'name':'host','hostPath':{'path':'/','type':'Directory'}},{'name':'source','configMap':{'name':'etcd-ssd-source'}}]}}}}
print(json.dumps(job))
