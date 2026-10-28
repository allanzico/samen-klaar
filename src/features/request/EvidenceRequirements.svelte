<script lang="ts">
 import type {AppState,Job,Evidence} from '../../lib/types';
 import {api} from '../../lib/api';
 import {workflowApi} from '../../lib/workflow-api';
 import {t} from '../../lib/i18n.svelte';
 let {app,job}:{app:AppState;job:Job}=$props();
 let busy=$state('');let error=$state('');let notes=$state<Record<string,string>>({});
 const round=$derived(job.completionRound||1);
 const writable=$derived(app.role==='professional'&&job.workStarted&&!job.completionApproved&&!job.paid);
 const current=(e:Evidence)=>e.scopeVersion===job.scopeVersion&&(e.completionRound||1)===round;
 function covered(id:string){return job.evidence.some(e=>current(e)&&e.workItemIds?.includes(id))||job.evidenceResolutions?.some(r=>r.workItemId===id&&r.scopeVersion===job.scopeVersion&&r.completionRound===round)}
 async function upload(id:string,event:Event){const file=(event.currentTarget as HTMLInputElement).files?.[0];if(!file||app.pending)return;busy=id;app.pending=true;error='';try{app.serverState=await api.uploadEvidence(file,app.workspaceId,[id]);}catch(e){error=e instanceof Error?e.message:'Upload failed';}finally{app.pending=false;busy='';}}
 async function note(id:string){if(!app.workspaceId||app.pending)return;busy=id;app.pending=true;error='';try{app.serverState=await workflowApi.note(app.workspaceId,notes[id]||'',[id]);notes[id]='';}catch(e){error=e instanceof Error?e.message:'Could not save the note';}finally{app.pending=false;busy='';}}
</script>
{#if job.workItems?.length}
 <section class="box requirements" aria-label={t('Agreed completion documentation')}>
  <h2 class="small-title">{t('Agreed completion documentation')}</h2>
  <p class="secondary-text">{t('Each requirement belongs to the agreed scope. Previous attempts stay in history.')}</p>
  <ul class="rows">
   {#each job.workItems as item(item.id)}
    <li class="requirement">
     <div><b>{item.description}</b><p class="meta">{covered(item.id)?t('Documentation supplied'):t('Documentation needed')} · {t('Scope {version}',{version:job.scopeVersion})}</p></div>
     {#if writable}
      {#if item.kind==='note'}
       <label class="field"><span>{t('Work note')}</span><textarea class="textarea" bind:value={notes[item.id]} minlength="12" maxlength="1200" disabled={app.pending}></textarea></label>
       <button type="button" class="btn btn-outline btn-sm" disabled={app.pending||(notes[item.id]||'').trim().length<12} onclick={()=>note(item.id)}>{t('Save work note')}</button>
      {:else}
       <label class="field"><span>{t('Upload documentation for this item')}</span><input type="file" accept={item.kind==='photo'?'image/jpeg,image/png,image/webp':item.kind==='document'?'application/pdf':'image/jpeg,image/png,image/webp,application/pdf'} disabled={app.pending} onchange={e=>upload(item.id,e)}/></label>
      {/if}
      {#if busy===item.id}<p role="status">{t('Saving…')}</p>{/if}
     {/if}
    </li>
   {/each}
  </ul>
  {#if job.evidenceReview?.coverage.some(f=>['unclear','not_shown'].includes(f.coverage))}<p class="secondary-text">{t('Flagged documentation needs a human review before completion can be approved.')}</p>{/if}
  {#if error}<p class="field-error" role="alert">{t(error)}</p>{/if}
 </section>
{/if}
<style>.requirements{display:grid;gap:12px;padding:20px 22px}.requirement{display:grid;gap:10px;padding-block:14px;overflow-wrap:anywhere}.requirement input{max-width:100%;font:inherit}.requirement button{justify-self:start}</style>
