from http.server import BaseHTTPRequestHandler, HTTPServer
import json, os
class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        payload=json.dumps({'backend':os.environ['BACKEND_NAME'],'path':self.path,'host':self.headers.get('Host'),'proto':self.headers.get('X-Forwarded-Proto'),'forwarded_host':self.headers.get('X-Forwarded-Host'),'forwarded_port':self.headers.get('X-Forwarded-Port'),'real_ip':self.headers.get('X-Real-IP')}).encode()
        self.send_response(200)
        self.send_header('Content-Type','application/json')
        self.send_header('Content-Length',str(len(payload)))
        self.end_headers()
        self.wfile.write(payload)
    def log_message(self,*args):pass
HTTPServer(('0.0.0.0',3000),Handler).serve_forever()
