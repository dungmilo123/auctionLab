import { useCallback, useEffect, useState } from 'react'
import { AccountError } from '@/api/account'
import { useAccount } from '@/account/AccountProvider'
import type { PrivateRead } from '@/routes/account/usePrivateRead'
// Requests are session-owned; an obsolete resolution cannot populate another account.
export function startParticipationPoll<T>(load: (signal: AbortSignal)=>Promise<T>, success: (v:T)=>void, failure: (e:AccountError)=>void, visible=()=>!document.hidden) {
  let live=true; let inFlight=false; let generation=0; let controller:AbortController|undefined
  const refresh=()=>{ if(!live||!visible())return; const g=++generation;inFlight=true;controller?.abort();controller=new AbortController();void load(controller.signal).then(v=>{if(live&&g===generation)success(v)},e=>{if(live&&g===generation)failure(e instanceof AccountError?e:new AccountError('unavailable'))}).finally(()=>{if(g===generation)inFlight=false}) }
  const poll=()=>{if(!inFlight)refresh()}
  const timer=setInterval(poll,2000);refresh()
  return {refresh,stop:()=>{live=false;generation++;controller?.abort();clearInterval(timer)}}
}
export function useParticipation<T>(load:(signal:AbortSignal)=>Promise<T>,revision=0){
  const {store}=useAccount();const [state,setState]=useState<PrivateRead<T>>({status:'loading'});const [attempt,setAttempt]=useState(0)
  const retry=useCallback(()=>setAttempt(n=>n+1),[])
  useEffect(()=>{
    const p=startParticipationPoll(load,data=>setState({status:'success',data}),error=>{if(error.status===401)store.signedOut();else setState({status:'error',error})})
    const hide=()=>{if(document.hidden){p.stop();setState({status:'loading'})}else retry()}
    const pagehide=()=>{p.stop();setState({status:'loading'})}
    window.addEventListener('focus',p.refresh);window.addEventListener('pagehide',pagehide);document.addEventListener('visibilitychange',hide)
    return()=>{p.stop();window.removeEventListener('focus',p.refresh);window.removeEventListener('pagehide',pagehide);document.removeEventListener('visibilitychange',hide)}
  },[load,revision,attempt,store,retry])
  return {state,retry}
}
