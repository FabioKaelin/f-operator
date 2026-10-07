"""Sequential bounded tests of real frontend images; never reads kubeconfig."""
import argparse, subprocess, time, urllib.request, uuid
p = argparse.ArgumentParser()
p.add_argument("--image", action="append", required=True)
a = p.parse_args()
for image in a.image:
    subprocess.run(["docker", "pull", image], check=True)
    for compatible in (False, True):
        name = "foperator-nginx-" + uuid.uuid4().hex[:10]
        command = ["docker", "run", "--detach", "--name", name, "--memory", "64m", "--memory-swap", "64m", "--cpus", "0.5", "--pids-limit", "64", "--security-opt", "no-new-privileges", "--cap-drop", "ALL", "--publish", "127.0.0.1::80"]
        if compatible:
            for cap in ("CHOWN", "SETUID", "SETGID", "NET_BIND_SERVICE"):
                command += ["--cap-add", cap]
        command += [image]
        try:
            subprocess.run(command, check=True, stdout=subprocess.DEVNULL)
            published = subprocess.run(["docker", "port", name, "80/tcp"], text=True, capture_output=True)
            address = published.stdout.strip()
            deadline = time.monotonic() + 20
            served = False
            while address and time.monotonic() < deadline:
                try:
                    with urllib.request.urlopen("http://" + address + "/", timeout=2) as response:
                        served = response.status == 200 and b"<html" in response.read().lower()
                    if served: break
                except Exception: pass
                if subprocess.check_output(["docker", "inspect", "--format", "{{.State.Running}}", name], text=True).strip() != "true": break
                time.sleep(.5)
            logs = subprocess.check_output(["docker", "logs", name], stderr=subprocess.STDOUT, text=True)
            if compatible:
                assert served, logs
                assert "Operation not permitted" not in logs and "[emerg]" not in logs, logs
                print("PASS compatibility profile: " + image, flush=True)
            else:
                print(("BASELINE serves HTTP: " if served else "BASELINE startup failed: ") + image, flush=True)
                if not served: print(logs[-1500:], flush=True)
        finally:
            subprocess.run(["docker", "rm", "--force", name], check=True, stdout=subprocess.DEVNULL)
