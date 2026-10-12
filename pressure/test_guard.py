import copy
import json
import unittest
from unittest.mock import patch
from guard import Guard
class API:
    def __init__(self):
        self.actions=[];self.running=True;self.dbready=False
        self.objects={
          '/apis/k8s.fabkli.ch/v1/namespaces/app/fdeployments/web':{'metadata':{'name':'web','resourceVersion':'1','generation':1},'spec':{'replicas':2}},
          '/apis/apps/v1/namespaces/app/deployments/web':{'metadata':{'name':'web','generation':1},'spec':{'replicas':2,'selector':{'matchLabels':{'app':'web'}}}},
          '/apis/apps/v1/namespaces/app/statefulsets/db':{'metadata':{'name':'db','resourceVersion':'1','generation':1},'spec':{'replicas':1,'selector':{'matchLabels':{'app':'db'}}},'status':{'readyReplicas':1,'observedGeneration':1}}
        }
    def call(self,path,method='GET',body=None):
        if 'configmaps/pressure-state' in path:return {'metadata':{'resourceVersion':'1'},'data':body.get('data',{}) if body else {'state.json':'{}'}}
        if '/pods?' in path:return {'items':[{}] if self.running else []}
        obj=self.objects[path]
        if method=='PATCH':
            self.actions.append((path,body['spec']))
            obj['spec'].update(body['spec']);obj['metadata'].setdefault('annotations',{}).update(body['metadata'].get('annotations',{}))
        return copy.deepcopy(obj)
class GuardTests(unittest.TestCase):
    def test_apps_before_database_and_database_before_apps(self):
        api=API();guard=Guard(api,{});state={};obj={'metadata':{'resourceVersion':'1'}};group={'name':'app','apps':[['fdeployments','app','web']],'databases':[['statefulsets','app','db']]}
        self.assertFalse(guard.suspend(obj,state,group))
        self.assertEqual(len(api.actions),1);self.assertTrue(api.actions[0][1]['suspend'])
        api.running=False;self.assertTrue(guard.suspend(obj,state,group));self.assertEqual(api.actions[1][1],{'replicas':0})
        dbpath='/apis/apps/v1/namespaces/app/statefulsets/db';api.objects[dbpath]['status']['readyReplicas']=0
        self.assertFalse(guard.resume(obj,state));self.assertEqual(api.actions[2][1],{'replicas':1});self.assertEqual(len(api.actions),3)
        api.objects[dbpath]['status']['readyReplicas']=1
        apppath='/apis/apps/v1/namespaces/app/deployments/web';api.objects[apppath]['status']={'readyReplicas':2,'observedGeneration':1}
        self.assertTrue(guard.resume(obj,state));self.assertEqual(api.actions[-1][1],{'suspend':False})
    def test_manual_disabled_app_stays_disabled(self):
        api=API();api.objects['/apis/k8s.fabkli.ch/v1/namespaces/app/fdeployments/web']['spec']['suspend']=True
        guard=Guard(api,{})
        self.assertFalse(guard.suspend({}, {}, {'name':'app','apps':[['fdeployments','app','web']]}));self.assertEqual(api.actions,[])
    def test_intervening_owner_or_replica_change_refused(self):
        api=API();guard=Guard(api,{});target=['statefulsets','app','db'];entry=guard.capture(target,'test')
        guard.mutate(target,entry,True)
        api.objects['/apis/apps/v1/namespaces/app/statefulsets/db']['spec']['replicas']=3
        with self.assertRaises(ValueError):guard.mutate(target,entry,False)
    def test_order_and_protected_infrastructure(self):
        from pathlib import Path
        policy=json.loads(Path(__file__).with_name('policy.json').read_text())
        self.assertEqual(policy['groups'][0]['name'],'shortcuts');self.assertEqual(policy['groups'][-1]['name'],'tipp-prod')
        self.assertFalse(any('prometheus' in t[2] or 'etcd' in t[2] for g in policy['groups'] for t in g['apps']+g.get('databases',[])))

    def test_recreated_database_is_not_adopted(self):
        api=API();guard=Guard(api,{});target=['statefulsets','app','db'];entry=guard.capture(target,'test')
        api.objects['/apis/apps/v1/namespaces/app/statefulsets/db']['metadata']['uid']='new-database'
        with self.assertRaises(ValueError):guard.mutate(target,entry,True)
        self.assertEqual(api.actions,[])
    def test_manually_suspending_app_must_terminate_before_database(self):
        api=API();api.objects['/apis/k8s.fabkli.ch/v1/namespaces/app/fdeployments/web']['spec']['suspend']=True
        guard=Guard(api,{})
        self.assertFalse(guard.suspend({'metadata':{'resourceVersion':'1'}},{},{'name':'app','apps':[['fdeployments','app','web']],'databases':[['statefulsets','app','db']]}))
        self.assertEqual(api.actions,[])

    def test_api_reloads_rotated_projected_token(self):
        import tempfile,io
        from pathlib import Path
        from guard import API as RealAPI
        class Opener:
            def __init__(self):self.tokens=[]
            def open(self,request,timeout):
                self.tokens.append(request.get_header('Authorization'))
                return io.BytesIO(b'{}')
        with tempfile.TemporaryDirectory() as directory:
            path=Path(directory)/'token';path.write_text('first-fixture')
            api=object.__new__(RealAPI);api.base='https://k8s.invalid';api.token_path=path;api.opener=Opener()
            api.call('/test');path.write_text('rotated-fixture');api.call('/test')
            self.assertEqual(api.opener.tokens,['Bearer first-fixture','Bearer rotated-fixture'])
