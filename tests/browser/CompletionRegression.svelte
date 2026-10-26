<script lang="ts">
  import RequestPage from '../../src/features/request/RequestPage.svelte';
  import {createAppState} from '../../src/state/app-state.svelte';
  import {setLocale} from '../../src/lib/i18n.svelte';
  import {emptyWorkspace} from './fixtures';
  import type {AppActions, WorkspaceState} from '../../src/lib/types';

  const params=new URLSearchParams(location.search);
  const mobile=params.has('mobile-check');
  const customer=params.has('customer');
  const storageKey=`completion-fixture-${customer?'customer':'professional'}`;
  const app=createAppState();
  app.role=customer?'customer':'professional';app.workspaceId='completion-fixture';
  app.session={configured:true,authenticated:true,activeRole:app.role,account:{id:customer?'fixture-customer':'fixture-professional'}};
  app.providerConfig={paymentsAvailable:false,evidenceUploadsAvailable:true,guidedReviewAvailable:true} as typeof app.providerConfig;
  const initial=emptyWorkspace('completion-fixture');
  Object.assign(initial.job,{id:'completion-fixture-job',title:'Repair the kitchen sink',description:'Repair a leaking sink connection',professional:'Test professional',professionalAccountId:'fixture-professional',professionalProfileId:'fixture-service',customer:'Test customer',quoteApproved:true,workStarted:true,detailAdded:true,completionRevision:customer});
  initial.job.quote={...initial.job.quote,totalMinor:12500,lines:[{label:'Repair the sink connection',amountMinor:12500,basis:'Agreed work'}]};
  initial.job.evidence=[{kind:'document',assetId:'synthetic-file',contentType:'application/pdf',scopeVersion:1,completionRound:1,label:'Synthetic completion file',description:'Fixture attachment; no real file is transmitted.'},{kind:'note',label:'Professional work note',description:'The work is complete.',scopeVersion:1,completionRound:1}];
  initial.intake={step:'selected',description:'Repair the kitchen sink',detail:'The connection leaks',category:'Plumbing',route:'catalog',question:'',options:[],messages:[],urgency:'Flexible',budget:'Show the price',matches:[],location:null,needsReview:false};
  let workspace=$state<WorkspaceState>(JSON.parse(sessionStorage.getItem(storageKey)||'null')||initial);
  $effect(()=>{app.serverState=workspace});
  function save(){sessionStorage.setItem(storageKey,JSON.stringify(workspace));app.serverState=workspace}
  function reset(){workspace=structuredClone(initial);app.error='';save()}
  const actions={act:async(action:string,value:string)=>{
    app.error='';
    if(action==='add-evidence'){
      workspace.job.evidence.push({kind:'note',label:'Professional work note',description:value,scopeVersion:1,completionRound:1});workspace.job.completionCheck=null;save();return true;
    }
    if(action==='submit-evidence'){
      app.pending=true;app.checkingCompletion=true;
      await new Promise(resolve=>setTimeout(resolve,300));
      if(params.has('offline')){app.error='We couldn’t check your photos and work note. Your draft is saved. Try again before sending it to the customer.';app.pending=false;app.checkingCompletion=false;return false}
      const note=[...workspace.job.evidence].reverse().find(item=>item.kind==='note')?.description||'';
      const documented=/replaced|seal|vervangen|afdichting/i.test(note);
      workspace.job.completionCheck={summary:documented?'The work note describes the repair and checks.':'Explain which connection was repaired and what was checked afterward.',coverage:[{scopeItem:'work-1',coverage:documented?'documented':'unclear',evidenceIds:['synthetic-file'],observation:'A generic statement that work is complete does not describe the agreed repair.'}],limitations:'Photos cannot confirm workmanship or safety.',reviewedAt:new Date().toISOString()};
      workspace.job.evidenceSubmitted=documented;workspace.job.completionRevision=!documented;
      app.pending=false;app.checkingCompletion=false;save();return true;
    }
    if(action==='revise-completion'){workspace.job.evidenceSubmitted=false;workspace.job.completionRevision=true;workspace.job.completionCheck=null;save();return true}
    return false;
  },refresh:async()=>{},dismissError:()=>{app.error=''},focusComposer:()=>{},closeProposal:()=>{app.composeMode='message'}} as unknown as AppActions;
  setLocale(params.has('nl')?'nl':'en',false);
</script>
{#if mobile}
 <main class="mobile-check"><h1>390px professional completion check</h1><iframe title="Mobile completion" width="390" height="740" src="?frame&nl"></iframe></main>
{:else}
 <nav class="fixture-controls"><span>Synthetic completion UI</span><button class="btn btn-quiet btn-sm" onclick={reset}>Reset fixture</button><a href="?nl">Nederlands</a><a href="?offline">Check unavailable</a><a href="?customer">Customer waiting</a></nav>
 <RequestPage {app} {actions} {workspace} onBack={()=>{}}/>
{/if}
<style>
 .fixture-controls{position:sticky;top:0;z-index:30;height:var(--sk-topbar-h);display:flex;align-items:center;gap:16px;padding:12px 20px;background:var(--sk-ground);white-space:nowrap;overflow:hidden;border-bottom:1px solid var(--sk-line);box-sizing:border-box}
 .mobile-check{padding:16px}.mobile-check iframe{border:1px solid var(--sk-line);margin-top:12px}
</style>
