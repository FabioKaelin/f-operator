import tempfile,unittest,pathlib,shutil
from unittest.mock import patch
import migrate_etcd
class MigrationTests(unittest.TestCase):
    def scenario(self,tamper=False):
        with tempfile.TemporaryDirectory() as folder:
            root=pathlib.Path(folder);ops=root/'ops';ops.mkdir();old=root/'old';new=root/'ssd/data';new.parent.mkdir()
            (old/'member/snap').mkdir(parents=True);(old/'member/snap/db').write_bytes(b'original-member-state');(old/'pre-ssd-snapshot.db').write_bytes(b'snapshot')
            manifest=root/'etcd.yaml';original='      path: /var/lib/minikube/etcd\n';manifest.write_text(original);calls=[]
            def run(*args):
                calls.append(args)
                if args[0]=='crictl' and args[-1]=='-q':return 'running-etcd' if not any(c[0]=='crictl' and 'stop' in c for c in calls) else ''
                if args[0]=='cp':
                    shutil.copytree(args[2],args[3])
                    if tamper:(pathlib.Path(args[3])/'snap/db').write_bytes(b'corrupted-copy')
                return ''
            with patch.multiple(migrate_etcd,root=ops,old=old,new=new,manifest=manifest),patch.object(migrate_etcd,'run',side_effect=run),patch('os.geteuid',return_value=0):
                if tamper:
                    with self.assertRaisesRegex(ValueError,'checksum'):migrate_etcd.main()
                    self.assertEqual(manifest.read_text(),original);self.assertFalse(new.exists())
                else:
                    migrate_etcd.main();self.assertIn('/var/lib/minikube/etcd-ssd/data',manifest.read_text())
                    self.assertEqual((new/'member/snap/db').read_bytes(),b'original-member-state')
                    calls.clear();migrate_etcd.main();self.assertFalse(any(c[:2]==('systemctl','stop') for c in calls))
            self.assertEqual((old/'member/snap/db').read_bytes(),b'original-member-state')
            if tamper:self.assertIn(('systemctl','start','kubelet'),calls)
    def test_same_member_copy_and_idempotent_repeat(self):self.scenario()
    def test_checksum_failure_keeps_manifest_and_restarts_kubelet(self):self.scenario(True)
if __name__=='__main__':unittest.main()
