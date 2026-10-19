"""Built-binary HTTP rehearsal against the explicitly provisioned disposable schema."""
import hashlib,json,os,urllib.request,urllib.error
from pathlib import Path
BASE='http://127.0.0.1:8080'
KEY=os.environ['ACCEPTANCE_ADMIN_KEY']
checks=[]
def request(method,path,body=None,auth=True,origin=None):
 headers={}
 if auth:headers['Cookie']='auction_admin='+KEY
 if body is not None:headers.update({'Content-Type':'application/json','X-AuctionLab-Admin':'1'})
 if origin is not None:headers['Origin']=origin
 req=urllib.request.Request(BASE+path,data=None if body is None else json.dumps(body).encode(),headers=headers,method=method)
 try:r=urllib.request.urlopen(req,timeout=10)
 except urllib.error.HTTPError as e:r=e
 raw=r.read();return r.status,json.loads(raw)
def get(path):
 status,body=request('GET',path);assert status==200,(status,body);return body['data']
def write(method,path,body,status=200):
 actual,result=request(method,path,body);assert actual==status,(actual,result);return result.get('data',result)
def clean(data):return {k:v for k,v in data.items() if k not in {'id','state','created_at','updated_at','demo','event_id','lot_ids','schedule'}}
root=Path(__file__).parent
if os.environ.get('ACCEPTANCE_VERIFY_RESTART'):
 status,actual=request('GET','/api/catalog/events',auth=False);assert status==200
 before=json.loads(Path('/tmp/auctionlab-public-before-restart.json').read_text());assert actual==before
 print(json.dumps({'status':'passed','restart_public_aggregate_equal':True}));raise SystemExit
for path in ['/api/admin/catalog/lots','/api/admin/catalog/events/missing/preview','/api/admin/catalog/nope']:
 assert request('GET',path,auth=False)[0]==401
wrong=KEY;KEY='wrong-test-cookie';assert request('GET','/api/admin/catalog/lots')[0]==401;KEY=wrong
assert request('GET','/api/admin/catalog/lots?k='+KEY,auth=False)[0]==401
assert request('POST','/api/admin/catalog/lots',{'title':'Rejected'},origin='https://foreign.example')[0]==403
checks.append('Missing/wrong cookie, query-key-only and foreign Origin rejected')
template=get('/api/admin/catalog/lots/demo-lot-03')
lots=[]
for number in [1,2]:
 created=write('POST','/api/admin/catalog/lots',{'title':f'Acceptance camera {number}'},201)
 content=clean(template);content.update({'title':f'Acceptance camera {number}','slug':created['slug'],'version':created['version'],'starting_price_minor':12550+number,'admin_notes':'ACCEPTANCE-PRIVATE-LOT-SENTINEL'})
 saved=write('PUT','/api/admin/catalog/lots/'+created['id'],content)
 assert saved['starting_price_minor']==12550+number and len(saved['images'])==2
 lots.append(saved)
event=write('POST','/api/admin/catalog/events',{'title':'Acceptance collection'},201)
source=get('/api/admin/catalog/events/demo-event-1');content=clean(source);content.update({'title':'Acceptance collection edited','slug':event['slug'],'version':event['version'],'admin_notes':'ACCEPTANCE-PRIVATE-EVENT-SENTINEL'})
event=write('PUT','/api/admin/catalog/events/'+event['id'],content)
checks.append('Created/edited two complete lots and an event with exact cents and ordered media')
event=write('PUT','/api/admin/catalog/events/'+event['id']+'/lots',{'version':event['version'],'lot_ids':[lots[0]['id'],lots[1]['id']]})
event=write('PUT','/api/admin/catalog/events/'+event['id']+'/lots',{'version':event['version'],'lot_ids':[lots[1]['id'],lots[0]['id']]})
preview=get('/api/admin/catalog/events/'+event['id']+'/preview')
assert [l['id'] for l in preview['lots']]==[lots[1]['id'],lots[0]['id']]
assert preview['lots'][1]['closes_at']-preview['lots'][0]['closes_at']==60000
lots[0]=get('/api/admin/catalog/lots/'+lots[0]['id']);broken=clean(lots[0]);broken['condition']='';broken=write('PUT','/api/admin/catalog/lots/'+lots[0]['id'],broken)
event=get('/api/admin/catalog/events/'+event['id'])
status,error=request('POST','/api/admin/catalog/events/'+event['id']+'/publish',{'version':event['version']});assert status==422 and 'lots.'+lots[0]['id']+'.condition' in error['error']['fields']
assert request('GET','/api/catalog/events/'+event['slug'],auth=False)[0]==404
corrected=clean(broken);corrected['condition']='Corrected known wear; demonstration illustration';write('PUT','/api/admin/catalog/lots/'+broken['id'],corrected)
event=get('/api/admin/catalog/events/'+event['id'])
published=write('POST','/api/admin/catalog/events/'+event['id']+'/publish',{'version':event['version']})
assert published['state']=='published' and len(published['lots'])==2
assert write('POST','/api/admin/catalog/events/'+event['id']+'/publish',{'version':0})==published
assert request('PUT','/api/admin/catalog/events/'+event['id'],clean(get('/api/admin/catalog/events/'+event['id'])))[0]==409
assert request('PUT','/api/admin/catalog/lots/'+lots[0]['id'],clean(get('/api/admin/catalog/lots/'+lots[0]['id'])))[0]==409
assert request('PUT','/api/admin/catalog/events/'+event['id']+'/lots',{'version':event['version'],'lot_ids':[]})[0]==409
checks.append('Saved/reordered membership, checked preview dates, corrected lot-specific publication failure, atomically published and rejected later edits')
status,public=request('GET','/api/catalog/events',auth=False);assert status==200
raw=json.dumps(public)
for secret in ['ACCEPTANCE-PRIVATE','admin_notes','version','created_at','truth_value','sealed_bids','token',KEY]:assert secret not in raw,secret
assert len(public['data'])==1
Path('/tmp/auctionlab-public-before-restart.json').write_text(json.dumps(public))
checks.append('Anonymous JSON excludes drafts/private metadata/game secrets; repeat publication returns identical aggregate')
report={'status':'passed','checks':checks,'published_event_id':event['id'],'published_slug':event['slug'],'ordered_lot_ids':[l['id'] for l in published['lots']],'opens_at':published['opens_at'],'planned_end':published['planned_end'],'public_digest':hashlib.sha256(raw.encode()).hexdigest()}
(root/'catalog-results.json').write_text(json.dumps(report,indent=2)+'\n')
print(json.dumps({'status':'passed','checks':len(checks)}))
