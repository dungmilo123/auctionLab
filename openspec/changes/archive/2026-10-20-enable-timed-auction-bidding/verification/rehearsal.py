"""HTTP drill against the explicitly configured disposable local test server.
Credentials come from the operator environment and are never written to evidence.
"""
import json, os, uuid, urllib.request, urllib.error, http.cookiejar, pathlib
BASE = 'http://127.0.0.1:8080'
OUT = pathlib.Path(__file__).parent

def client(email):
    c = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
    status, _ = request(c, '/api/auth/login', {'email':email, 'password':os.environ['REHEARSAL_PASSWORD']})
    assert status == 200
    return c

def request(c,path,body=None,method=None,discard=False):
    req=urllib.request.Request(BASE+path,data=json.dumps(body).encode() if body is not None else None,method=method or ('POST' if body is not None else 'GET'),headers={'Content-Type':'application/json','X-AuctionLab-Account':'1','Origin':BASE})
    try: response=c.open(req,timeout=6)
    except urllib.error.HTTPError as e: response=e
    status=response.status
    if discard: response.close();return status,None
    with response: return status,json.load(response)

a=client('bidder-one@demo.auctionlab.test');b=client('bidder-two@demo.auctionlab.test')
public=urllib.request.build_opener()
_,s=request(public,'/api/auctions/lots/demo-lot-07');opening=s['data']['next_minimum_minor'];intent={'lot_id':'demo-lot-07','amount_minor':opening,'request_id':str(uuid.uuid4())}
assert request(a,'/api/account/bids',intent,discard=True)[0]==201
status,replay=request(a,'/api/account/bids',intent);assert status==200 and replay['data']['receipt']['replay']
_,after=request(public,'/api/auctions/lots/demo-lot-07');assert after['data']['bid_count']==1
selfraise={**intent,'amount_minor':opening+500,'request_id':str(uuid.uuid4())};status,refusal=request(a,'/api/account/bids',selfraise);assert status==409 and refusal['error']['code']=='already_leading'
low={**intent,'request_id':str(uuid.uuid4())};status,refusal=request(b,'/api/account/bids',low);assert status==409 and refusal['error']['code']=='minimum_not_met'
status,_=request(a,'/api/account/watchlist/demo-lot-07',{},'PUT');assert status==200
_,pa=request(a,'/api/account/bids/demo-lot-07');_,pb=request(b,'/api/account/bids/demo-lot-07');assert pa['data']['status']=='leading' and len(pa['data']['bids'])==1 and pb['data']['status']=='not_participating' and pb['data']['bids']==[]
report={'discarded_accepted_response_replayed':True,'durable_bid_count':after['data']['bid_count'],'self_leading_refused':True,'below_minimum_refused':True,'private_isolation':True,'watchlist_saved_independently':True}
(OUT/'http-rehearsal.json').write_text(json.dumps(report,indent=2)+'\n');print(json.dumps(report))
