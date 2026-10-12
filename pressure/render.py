"""Render guard source/config and narrow namespace-specific RBAC from Git."""
import json
import hashlib
from pathlib import Path
root=Path(__file__).parent
policy=json.loads((root/'policy.json').read_text());namespace='f-operator-system';sa='pressure-guard'
items=json.loads((root/'priorities.json').read_text())['items']
items.extend([
 {'apiVersion':'v1','kind':'ServiceAccount','metadata':{'name':sa,'namespace':namespace}},
 {'apiVersion':'v1','kind':'ConfigMap','metadata':{'name':'pressure-config','namespace':namespace},'data':{'policy.json':json.dumps(policy),'guard.py':(root/'guard.py').read_text()}},
 {'apiVersion':'rbac.authorization.k8s.io/v1','kind':'Role','metadata':{'name':sa,'namespace':namespace},'rules':[{'apiGroups':[''],'resources':['configmaps'],'resourceNames':['pressure-state'],'verbs':['get','patch']}]},
 {'apiVersion':'rbac.authorization.k8s.io/v1','kind':'RoleBinding','metadata':{'name':sa,'namespace':namespace},'roleRef':{'apiGroup':'rbac.authorization.k8s.io','kind':'Role','name':sa},'subjects':[{'kind':'ServiceAccount','name':sa,'namespace':namespace}]},
 {'apiVersion':'apps/v1','kind':'Deployment','metadata':{'name':sa,'namespace':namespace},'spec':{'replicas':1,'strategy':{'type':'Recreate'},'selector':{'matchLabels':{'app':sa}},'template':{'metadata':{'labels':{'app':sa},'annotations':{'fabkli.ch/pressure-source':hashlib.sha256((root/'guard.py').read_bytes()+(root/'policy.json').read_bytes()).hexdigest()}},'spec':{'serviceAccountName':sa,'priorityClassName':'fabkli-infrastructure','securityContext':{'runAsNonRoot':True,'runAsUser':101,'runAsGroup':101,'seccompProfile':{'type':'RuntimeDefault'}},'containers':[{'name':'guard','image':'python@sha256:0aecac02dc3d4c5dbb024b753af084cafe41f5416e02193f1ce345d671ec966e','command':['python','-u','/config/guard.py'],'env':[{'name':'PYTHONDONTWRITEBYTECODE','value':'1'}],'resources':{'requests':{'cpu':'20m','memory':'32Mi'},'limits':{'cpu':'100m','memory':'96Mi'}},'securityContext':{'allowPrivilegeEscalation':False,'readOnlyRootFilesystem':True,'capabilities':{'drop':['ALL']}},'volumeMounts':[{'name':'config','mountPath':'/config','readOnly':True},{'name':'proc','mountPath':'/host-proc','readOnly':True},{'name':'cgroup','mountPath':'/host-cgroup','readOnly':True}]}],'volumes':[{'name':'config','configMap':{'name':'pressure-config','defaultMode':292}},{'name':'proc','hostPath':{'path':'/proc','type':'Directory'}},{'name':'cgroup','hostPath':{'path':'/sys/fs/cgroup','type':'Directory'}}]}}}},
])
# Minikube's container filesystem needs readonly bind mounts of outer proc/cgroup;
# deployment preflight refuses to install a blind guard if these are absent.
for ns in sorted({t[1] for g in policy['groups'] for t in g['apps']+g.get('databases',[])}):
 targets=[t for g in policy['groups'] for t in g['apps']+g.get('databases',[]) if t[1]==ns]
 rules=[{'apiGroups':[''],'resources':['pods'],'verbs':['get','list']}]
 for kind in ('fdeployments','deployments','statefulsets'):
  names=sorted({t[2] for t in targets if t[0]==kind or kind=='deployments' and t[0]=='fdeployments'})
  if names:rules.append({'apiGroups':['k8s.fabkli.ch' if kind=='fdeployments' else 'apps'],'resources':[kind],'resourceNames':names,'verbs':['get','patch'] if any(t[0]==kind for t in targets) else ['get']})
 items.extend([{'apiVersion':'rbac.authorization.k8s.io/v1','kind':'Role','metadata':{'name':sa,'namespace':ns},'rules':rules},{'apiVersion':'rbac.authorization.k8s.io/v1','kind':'RoleBinding','metadata':{'name':sa,'namespace':ns},'roleRef':{'apiGroup':'rbac.authorization.k8s.io','kind':'Role','name':sa},'subjects':[{'kind':'ServiceAccount','name':sa,'namespace':namespace}]}])
print(json.dumps({'apiVersion':'v1','kind':'List','items':items},indent=2))
