<script lang="ts">
  import ProfessionalProfileEditor from '../../src/features/professional/ProfessionalProfileEditor.svelte';
  import CustomerProfile from '../../src/features/customer/CustomerProfile.svelte';
  import FreeTimes from '../../src/features/professional/FreeTimes.svelte';
  import IntakeStage from '../../src/features/customer/IntakeStage.svelte';
  import RequestDialogs from '../../src/features/request/RequestDialogs.svelte';
  import { createAppState } from '../../src/state/app-state.svelte';
  import { emptyWorkspace } from './fixtures';
  import type { AppActions, ProfessionalProfile, Intake } from '../../src/lib/types';
  let view = $state('professional');
  let result = $state('');
  const profile: ProfessionalProfile = {id:'fixture-pro',status:'active',displayName:'Fixture plumber',category:'Plumbing',serviceName:'Fixture service',serviceDescription:'Repair visit',serviceArea:'Amsterdam',baseLocation:{id:'place',source:'mapbox',kind:'postcode',label:'Amsterdam',latitude:52.37,longitude:4.9},travelRadiusKm:25,latitude:52.37,longitude:4.9,currency:'eur',priceMinor:10000,calloutMinor:0,durationMinutes:60,materialsPolicy:'Materials extra',exclusions:[]};
  let customer = {displayName:'Fixture customer',email:'fixture@example.test',phone:'',location:null};
  const realFetch = window.fetch;
  window.fetch = async (input, init) => {
    const path = String(input);
    let body: unknown;
    if(path==='/api/customer/profile') {if(init?.method==='PUT'){customer={...customer,...JSON.parse(String(init.body))};result='PASS: customer profile saved';} body=customer;}
    else if(path==='/api/professional/profile') {if(init?.method==='POST')result='PASS: professional profile saved';body={profile};}
    else if(path==='/api/professional/schedule') body={schedule:{timezone:'Europe/Amsterdam',visitMinutes:60,gapMinutes:30,noticeMinutes:60,horizonDays:14,week:[{weekday:1,start:'09:00',end:'17:00'}]},daysOff:[],weeklyCount:5};
    else if(path.startsWith('/api/professional/availability')) body={slots:[],removed:[],daysOff:[]};
    else return realFetch(input,init);
    return new Response(JSON.stringify(body),{headers:{'Content-Type':'application/json'}});
  };
  const app = createAppState();
  app.role = 'customer'; app.workspaceId = 'fixture-waitlist';
  app.session = {configured:true,authenticated:true,account:{id:'fixture-customer',displayName:'Fixture customer'}};
  const actions: Partial<AppActions> = {deleteRequest:async()=>{result='PASS: disposable waitlist request deleted';return true;}};
  const intake: Intake = {step:'waitlist',waitlisted:true,category:'Plumbing',description:'Disposable waitlist fixture',location:{label:'Fixture area',source:'mapbox',latitude:52.37,longitude:4.9},messages:[],matches:[],options:[],route:'direct',question:'',detail:'',urgency:'',budget:'',needsReview:false};
  const workspace = {...emptyWorkspace('fixture-waitlist'),intake};
</script>
<main style="max-width:900px;margin:30px auto;padding:20px">
  <h1>Profile and schedule regression checks</h1>
  <nav style="display:flex;gap:12px;flex-wrap:wrap;margin:20px 0">{#each ['professional','customer','schedule','waitlist'] as tab}<button class="btn btn-outline" onclick={()=>{view=tab;result='';}}>{tab}</button>{/each}</nav>
  {#if result}<p role="status">{result}</p>{/if}
  {#if view==='professional'}<ProfessionalProfileEditor {profile} onSaved={()=>{result='PASS: professional profile saved';}}/>
  {:else if view==='customer'}<CustomerProfile {app}/>
  {:else if view==='schedule'}<FreeTimes/>
  {:else}<IntakeStage {app} actions={actions as AppActions} {intake}/><RequestDialogs {app} actions={actions as AppActions} {workspace} work={{changeDescription:'',changePrice:''}} historyOpen={false}/>{/if}
</main>
