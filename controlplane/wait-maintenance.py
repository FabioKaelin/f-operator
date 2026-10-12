"""Bounded GET polling through a control-plane storage maintenance interval."""
import subprocess,json,sys,time
end=time.monotonic()+660
while time.monotonic()<end:
    result=subprocess.run(['kubectl','--request-timeout=8s','get','job',sys.argv[1],'-n','f-operator-system','-o','json'],capture_output=True,text=True)
    if result.returncode==0:
        status=json.loads(result.stdout).get('status',{})
        if status.get('failed'):raise SystemExit('Host maintenance failed; retained node journal and data need review')
        if status.get('succeeded'):
            ready=subprocess.run(['kubectl','--request-timeout=8s','get','--raw','/readyz'],capture_output=True,text=True)
            if ready.returncode==0 and ready.stdout.strip()=='ok':print('Host maintenance and API readiness passed');break
    time.sleep(5)
else:raise SystemExit('Host/API readiness deadline reached; inspect retained maintenance journal')
