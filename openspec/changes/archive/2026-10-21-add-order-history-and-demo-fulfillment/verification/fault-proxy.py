import http.server,http.client,json,pathlib,time
control=pathlib.Path('/tmp/auctionlab-fault-mode.json')
class Handler(http.server.BaseHTTPRequestHandler):
 def log_message(self,*args):pass
 def run(self):
  body=self.rfile.read(int(self.headers.get('Content-Length','0')))
  mode=json.loads(control.read_text()) if control.exists() else {}
  private=self.path.startswith('/api/admin/orders')
  if private and self.command=='GET' and mode.get('deny'):
   data=b'{"error":{"code":"access_required"}}';self.send_response(401);self.send_header('Content-Type','application/json');self.send_header('Cache-Control','no-store');self.send_header('Content-Length',str(len(data)));self.end_headers();self.wfile.write(data);return
  if private and self.command=='GET' and mode.get('fail_reads'):
   data=b'{"error":{"code":"unavailable"}}';self.send_response(503);self.send_header('Content-Type','application/json');self.send_header('Cache-Control','no-store');self.send_header('Content-Length',str(len(data)));self.end_headers();self.wfile.write(data);return
  conn=http.client.HTTPConnection('127.0.0.1',8080,timeout=10)
  headers=dict(self.headers);conn.request(self.command,self.path,body=body,headers=headers);res=conn.getresponse();data=res.read();status=res.status;outheaders=res.getheaders();conn.close()
  if private and self.command=='POST' and self.path.endswith('/transition') and mode.get('lose_transition') and status==200:
   mode['lose_transition']=False;mode['fail_reads']=True;control.write_text(json.dumps(mode));status=503;data=b'{"error":{"code":"commit_unknown"}}';outheaders=[('Content-Type','application/json'),('Cache-Control','no-store')]
  if private and self.command=='GET' and mode.get('delay_id') and mode['delay_id'] in self.path:time.sleep(2)
  if private and self.command=='POST' and mode.get('delay_transition_id') and mode['delay_transition_id'] in self.path:time.sleep(2)
  self.send_response(status)
  for key,value in outheaders:
   if key.lower() not in ('content-length','transfer-encoding','connection','content-encoding'):self.send_header(key,value)
  self.send_header('Content-Length',str(len(data)));self.end_headers();self.wfile.write(data)
 do_GET=run
 do_POST=run
 def do_HEAD(self):self.run()
http.server.ThreadingHTTPServer(('127.0.0.1',8181),Handler).serve_forever()
