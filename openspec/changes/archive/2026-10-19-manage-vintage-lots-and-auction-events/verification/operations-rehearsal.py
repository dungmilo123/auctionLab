"""Disposable-instance outage, ownership, backup/restore and rollback evidence."""
import hashlib,json,os,shutil,signal,subprocess,time,urllib.request,urllib.error
from pathlib import Path
ROOT=next(parent for parent in Path(__file__).resolve().parents if (parent / "go.mod").is_file())
OUT=Path(__file__).parent
PG_URL=os.environ['TEST_DATABASE_URL']
SCHEMA=Path('/tmp/auctionlab-acceptance-schema').read_text().strip()
IMPORT_SCHEMA=Path('/tmp/auctionlab-import-schema').read_text().strip()
SOURCE=Path('/tmp/auctionlab-legacy-cutover-fixture/snapshot.db')
CONTAINER='auctionlab-change2-pg18'
ROLE='auctionlab_test'
DB='auctionlab_test'
checks=[]
def run(args,**kwargs):return subprocess.run(args,check=True,capture_output=True,**kwargs)
def sql(query,database=DB):return run(['docker','exec',CONTAINER,'psql','-U',ROLE,'-d',database,'-Atc',query],text=True).stdout.strip()
def request(port,path='/api/health',method='GET',body=None,key=None):
 headers={}
 if key:headers['Cookie']='auction_admin='+key
 if body is not None:headers.update({'Content-Type':'application/json','X-AuctionLab-Admin':'1'})
 r=urllib.request.Request(f'http://127.0.0.1:{port}'+path,data=None if body is None else json.dumps(body).encode(),headers=headers,method=method)
 try:r=urllib.request.urlopen(r,timeout=3)
 except urllib.error.HTTPError as e:r=e
 return r.status,r.read()
def wait_health(port,want=200):
 for _ in range(100):
  try:
   status,body=request(port)
   if status==want:return json.loads(body)
  except (OSError,ValueError):pass
  time.sleep(.1)
 raise AssertionError('bounded readiness did not reach expected status')
def process_for(port):
 rows=run(['ps','-eo','pid,args'],text=True).stdout.splitlines()
 matches=[int(row.strip().split()[0]) for row in rows if row.strip().endswith(f'./bin/auctionlab --port {port}')]
 assert len(matches)==1,matches
 return matches[0]
def stop(port):
 pid=process_for(port);os.kill(pid,signal.SIGINT)
 for _ in range(100):
  try:os.kill(pid,0)
  except ProcessLookupError:return
  time.sleep(.05)
 raise AssertionError('owned acceptance child did not stop')
def start(schema,port,database=DB,legacy=False):
 env=os.environ.copy();env.pop('DB_PATH',None);env.pop('ADMIN_KEY',None)
 if legacy:
  env['DB_PATH']='/tmp/auctionlab-rollback-working.db';command=['/tmp/auctionlab-before-postgresql','--port',str(port)]
 else:
  from urllib.parse import urlparse,urlunparse
  u=urlparse(PG_URL);env['DATABASE_URL']=urlunparse(u._replace(path='/'+database));env['DATABASE_SCHEMA']=schema
  if schema==SCHEMA:env['ADMIN_KEY']=os.environ['ACCEPTANCE_ADMIN_KEY']
  command=['./bin/auctionlab','--port',str(port)]
 log=open(f'/tmp/auctionlab-operation-{port}.log','wb')
 p=subprocess.Popen(command,env=env,stdout=log,stderr=log,cwd=ROOT);log.close();return p
original=hashlib.sha256(SOURCE.read_bytes()).hexdigest()
# The fixture's source has a live expired legacy run. It was independently verified before boot.
state=sql(f'SELECT phase,revision,budget,total_value FROM "{IMPORT_SCHEMA}".game CROSS JOIN "{IMPORT_SCHEMA}".players')
assert state=='SETTLED|5|1250000|350000',state
status,raw=request(8182,'/api/admin/catalog/lots',key=os.environ['LEGACY_ACCEPTANCE_KEY']);assert status==200
checks.append('Verified preboot import resumed expired English run: LIVE revision 4 -> SETTLED revision 5; legacy game budget 1000000 -> 1250000 (truth value 350000 minus bid 100000), total value 350000; imported capability still authorizes')
env=os.environ.copy();env['DATABASE_URL']=PG_URL;env['DATABASE_SCHEMA']=IMPORT_SCHEMA;env.pop('DB_PATH',None);env.pop('ADMIN_KEY',None)
competing=subprocess.run(['./bin/auctionlab','--seed-catalog','--seed-at','2030-01-01T00:00:00Z'],env=env,capture_output=True,text=True,timeout=10)
assert competing.returncode!=0 and 'writer ownership unavailable' in competing.stderr
verify=subprocess.run(['./bin/auctionlab','--import-sqlite',str(SOURCE),'--verify-only'],env=env,capture_output=True,text=True,timeout=10)
assert verify.returncode!=0 and 'verification mismatch: players' in verify.stderr
checks.append('Competing maintenance writer refused; independent verify-only detects expected post-boot settlement drift instead of trusting marker')
# Preserve complete published aggregate across app restart and repeat seed.
before=json.loads(request(8080,'/api/catalog/events')[1])
stop(8080)
env['DATABASE_SCHEMA']=SCHEMA
replay=run(['./bin/auctionlab','--seed-catalog','--seed-at','2040-01-01T00:00:00Z'],env=env,text=True)
assert json.loads(replay.stdout)['status']=='already_applied'
app=start(SCHEMA,8080);wait_health(8080)
after=json.loads(request(8080,'/api/catalog/events')[1]);assert before==after
checks.append('Built application restart and seed replay preserve published values/order/dates; ordinary startup does not seed or rebase')
# Backup contains secrets, so keep it outside the report and restrict file permissions.
backup=Path('/tmp/auctionlab-disposable-backup.dump')
with backup.open('wb') as f:subprocess.run(['docker','exec',CONTAINER,'pg_dump','-U',ROLE,'-d',DB,'-n',SCHEMA,'-Fc'],stdout=f,check=True)
backup.chmod(0o600)
restore_db='auctionlab_restore_'+SCHEMA[-12:]
run(['docker','exec',CONTAINER,'createdb','-U',ROLE,restore_db])
with backup.open('rb') as f:subprocess.run(['docker','exec','-i',CONTAINER,'pg_restore','-U',ROLE,'-d',restore_db,'--no-owner','--exit-on-error'],stdin=f,check=True,capture_output=True)
restored=start(SCHEMA,8183,database=restore_db);wait_health(8183)
assert json.loads(request(8183,'/api/catalog/events')[1])==before
restored.send_signal(signal.SIGINT);restored.wait(timeout=10)
checks.append('pg_dump/pg_restore into a disposable database/schema preserves identical public aggregate')
# Outage on this private PostgreSQL container only. No operational service is touched.
run(['docker','stop','--time','2',CONTAINER]);unavailable=wait_health(8080,503)
assert unavailable=={'status':'unavailable','database':'postgresql','ready':False}
status,body=request(8080,'/api/admin/catalog/lots','POST',{'title':'Outage must not save'},os.environ['ACCEPTANCE_ADMIN_KEY']);assert status==503,(status,body)
assert b'postgres://' not in body and b'password' not in body
run(['docker','start',CONTAINER])
for _ in range(100):
 try:
  if sql('SELECT 1')=='1':break
 except subprocess.CalledProcessError:time.sleep(.1)
else:raise AssertionError('PostgreSQL restart unavailable')
# Connection loss must not silently acquire a new owner for stale memory.
assert request(8080)[0]==503
app.send_signal(signal.SIGINT);app.wait(timeout=10)
app=start(SCHEMA,8080);wait_health(8080)
assert json.loads(request(8080,'/api/catalog/events')[1])==before
assert sql(f"SELECT count(*) FROM \"{SCHEMA}\".catalog_lots WHERE title='Outage must not save'")=='0'
checks.append('PostgreSQL outage returns health/save 503; restart does not reacquire stale owner; explicit app restart restores readiness with unchanged publication and no fabricated save')
# Prior SQLite binary uses a working copy; source is never opened by a writer.
copy=Path('/tmp/auctionlab-rollback-working.db')
assert not copy.exists();shutil.copy2(SOURCE,copy)
prior=start('',8184,legacy=True);wait_health(8184)
status,body=request(8184,'/admin?k='+os.environ['LEGACY_ACCEPTANCE_KEY']);assert status==200
prior.send_signal(signal.SIGINT);prior.wait(timeout=10)
assert hashlib.sha256(SOURCE.read_bytes()).hexdigest()==original
checks.append('Preserved prior binary boots/authorizes a working SQLite copy; fixed source checksum unchanged; PostgreSQL backup preserved; no reverse synchronization attempted')
report={'status':'passed','checks':checks,'source_snapshot_sha256':original,'rollback_limit':'Post-cutover PostgreSQL catalog edits/publication are absent from the prior SQLite snapshot. Reverting to it discards those writes unless separately retained; backup preserved. No automatic reverse synchronization.','backup_location':'private temporary backup outside repository','restored_schema_disposable':True}
(OUT/'operations-results.json').write_text(json.dumps(report,indent=2)+'\n')
# Leave the acceptance app available only until remaining browser/final checks finish.
Path('/tmp/auctionlab-final-acceptance-pid').write_text(str(app.pid))
print(json.dumps({'status':'passed','checks':len(checks)}))
