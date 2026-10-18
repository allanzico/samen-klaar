<script lang="ts">
 import{onMount}from'svelte';import{workflowApi,type ParentRequest}from'../../lib/workflow-api';import type{AppState,AppActions}from'../../lib/types';import{t}from'../../lib/i18n.svelte';
 let{app,actions}:{app:AppState;actions:AppActions}=$props();let parent=$state<ParentRequest|null>(null);let description=$state('');let adding=$state(false);let error=$state('');let busy=$state(false);let planID=crypto.randomUUID();
 onMount(()=>{if(app.workspaceId)void workflowApi.parent(app.workspaceId).then(p=>parent=p).catch(()=>undefined)});
 async function add(){if(!app.workspaceId)return;busy=true;error='';try{parent=await workflowApi.addJob(app.workspaceId,planID,description);description='';planID=crypto.randomUUID();adding=false;await actions.refresh();}catch(e){error=e instanceof Error?e.message:'Could not add a job';}finally{busy=false}}
</script>
{#if app.role==='customer'&&parent}
 <section class="box related" aria-label={t('Jobs in this request')}>
  <h2 class="small-title">{t('Jobs in this request')}</h2>
  {#if parent.jobs.length>1}<ul class="rows">{#each parent.jobs as job(job.workspace.id)}<li><button type="button" class="btn btn-quiet" disabled={busy||app.pending} onclick={()=>actions.selectRequest(job.workspace.id)} aria-current={job.workspace.id===app.workspaceId?'page':undefined}>{job.workspace.title}</button><p class="meta">{t(job.workspace.statusLabel)}{job.professional?` · ${job.professional}`:''}</p></li>{/each}</ul>{/if}
  <p class="secondary-text">{t('Each job has its own professional, agreement, visit, evidence and payment. Other professionals cannot see it.')}</p>
  {#if adding}<form onsubmit={e=>{e.preventDefault();void add()}}><label class="field"><span>{t('Describe the separate job')}</span><textarea class="textarea" bind:value={description} required minlength="8" maxlength="1000" disabled={busy}></textarea></label><div class="row"><button class="btn btn-outline" disabled={busy}>{t('Add job plan')}</button><button type="button" class="btn btn-quiet" disabled={busy} onclick={()=>adding=false}>{t('Cancel')}</button></div></form>{:else}<button class="btn btn-quiet" type="button" disabled={busy||parent.jobs.length>=8} onclick={()=>adding=true}>{t('Add another trade or job')}</button>{/if}
  {#if parent.jobs.length>1}<details class="disclosure"><summary>{t('Shared request timeline')}</summary><ol class="rows">{#each parent.timeline as event,i(i)}<li><b>{event.jobTitle}</b><p>{event.event.text}</p><span class="meta">{event.event.at||event.event.when}</span></li>{/each}</ol></details>{/if}
  {#if error}<p class="field-error" role="alert">{t(error)}</p>{/if}
 </section>
{/if}
<style>.related{display:grid;gap:12px;padding:20px 22px}.related form{display:grid;gap:12px}.related li{padding-block:8px;overflow-wrap:anywhere}</style>
