import type { WorkspaceState, EvidenceRequirement } from './types';
export type CaseNote = {id:number;recipientRole?:string;note:string;createdAt:string};
export type CaseDetail = {case:{kind:string;description:string};state:WorkspaceState;assignedTo?:string;status:string;outcome?:string;notes:CaseNote[];decisions:{actor:string;action:string;scopeVersion:number;at:string}[];paymentStatus:string};
export type EvidencePolicy = {profileId:string;requirements:EvidenceRequirement[];proposed?:EvidenceRequirement[];status:string;reviewedAt?:string};
export type CoordinationTask = {id:string;scopeVersion:number;round:number;recipientRole:string;status:string;question:string;reply:string;summary:string;proposedAction:string;outcome:string};
export type ParentRequest = {id:string;description:string;jobs:{workspace:{id:string;title:string;statusLabel:string;status:string};scopeVersion:number;totalMinor:number;currency:string;professional:string}[];timeline:{workspaceId:string;jobTitle:string;event:{text:string;when:string;at?:string}}[]};
async function request<T>(url:string, body?:unknown):Promise<T>{const response=await fetch(url,body===undefined?undefined:{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(body)});if(!response.ok)throw new Error(await response.text());return response.json();}
const caseURL=(id:string,kind:string)=>`/api/operator/cases/${encodeURIComponent(id)}?kind=${encodeURIComponent(kind)}`;
export const workflowApi={
 caseDetail:(id:string,kind:string)=>request<CaseDetail>(caseURL(id,kind)),
 caseAction:(id:string,kind:string,action:string,note='',recipient='',workItemID='')=>request<CaseDetail>(caseURL(id,kind),{action,note,recipient,workItemID}),
 support:(id:string)=>request<{notes:CaseNote[]}>(`/api/support/messages?workspaceId=${encodeURIComponent(id)}`),
 policy:()=>request<EvidencePolicy>('/api/professional/evidence-policy'),
 savePolicy:(requirements:EvidenceRequirement[])=>request<EvidencePolicy>('/api/professional/evidence-policy',{requirements}),
 policies:()=>request<{policies:EvidencePolicy[]}>('/api/operator/evidence-policies'),
 reviewPolicy:(profileId:string,approve:boolean)=>request<EvidencePolicy>('/api/operator/evidence-policies',{profileId,approve}),
 note:(workspaceId:string,note:string,workItemIds:string[])=>request<WorkspaceState>('/api/actions',{workspaceId,action:'add-evidence',value:note,workItemIds}),
 coordination:(id:string)=>request<{tasks:CoordinationTask[]}>(`/api/coordination?workspaceId=${encodeURIComponent(id)}`),
 reply:(id:string,taskID:string,reply:string)=>request<{tasks:CoordinationTask[]}>(`/api/coordination?workspaceId=${encodeURIComponent(id)}`,{taskID,reply}),
 parent:(id:string)=>request<ParentRequest>(`/api/request-plan?workspaceId=${encodeURIComponent(id)}`),
 addJob:(id:string,planID:string,description:string)=>request<ParentRequest>(`/api/request-plan?workspaceId=${encodeURIComponent(id)}`,{jobs:[{id:planID,description}]}),
};
