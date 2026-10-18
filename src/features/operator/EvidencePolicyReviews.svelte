<script lang="ts">
 import{onMount}from'svelte';import{workflowApi,type EvidencePolicy}from'../../lib/workflow-api';import{showToast}from'../../ui/toast-state.svelte';import{t}from'../../lib/i18n.svelte';
 let policies=$state<EvidencePolicy[]>([]);let error=$state('');let busy=$state('');
 async function load(){try{policies=(await workflowApi.policies()).policies}catch(e){error=e instanceof Error?e.message:'Could not load evidence policies'}}
 async function review(policy:EvidencePolicy,approve:boolean){busy=policy.profileId;error='';try{await workflowApi.reviewPolicy(policy.profileId,approve);showToast(approve?t('Evidence requirements approved'):t('Evidence requirements declined'));await load()}catch(e){error=e instanceof Error?e.message:'Could not review policy'}finally{busy=''}}
 onMount(()=>{void load()});
</script>
{#if policies.length||error}<section class="policy-reviews box"><h2 class="small-title">{t('Service evidence policies awaiting review')}</h2>{#if error}<p class="field-error" role="alert">{t(error)}</p>{/if}{#each policies as policy(policy.profileId)}<article><p class="meta">{t('Service profile')} · {policy.profileId}</p><ul class="rows">{#each policy.proposed||[] as requirement(requirement.id)}<li>{requirement.label} · {t(requirement.kind)}</li>{/each}</ul><div class="row"><button class="btn btn-outline btn-sm" type="button" disabled={!!busy} onclick={()=>review(policy,true)}>{t('Approve evidence requirements')}</button><button class="btn btn-quiet" type="button" disabled={!!busy} onclick={()=>review(policy,false)}>{t('Decline')}</button></div></article>{/each}</section>{/if}
<style>.policy-reviews{display:grid;gap:16px;padding:20px 22px}.policy-reviews article{display:grid;gap:10px;overflow-wrap:anywhere}.policy-reviews li{padding-block:6px}</style>
