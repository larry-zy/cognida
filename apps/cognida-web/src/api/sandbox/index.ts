import { http } from '@/utils/request'
import { withIdempotency } from '@/utils/idempotent'
import type { RefID } from '@/api/lifecycle'
export interface Limits {cpu_millis:number;memory_bytes:number;disk_bytes:number;processes:number;timeout_seconds:number;input_bytes:number;output_bytes:number;output_files:number;log_bytes:number}
export interface Profile {ID:string;Version:number;Image:string;IsolationLevel:string;MaxResources:Limits}
export interface InputMetadata {TenantID:number;Ref:RefID;SHA256:string;SizeBytes:number}
export interface SandboxRun {id:string;tenant_id:number;user_id:number;request_sha256:string;state:string;phase:string;version:number;cancel_requested:boolean;error_code?:string;deadline:string;output_refs?:RefID[];manifest?:{artifacts:{path:string;sha256:string;size_bytes:number;media_type:string}[]}}
export const sandboxApi = {
 profiles:()=>http.get<{enabled:boolean;reason?:string;profile?:Profile}>('/sandbox/profiles'),
 metadata:(ref:RefID)=>http.get<InputMetadata>(`/sandbox/inputs/${encodeURIComponent(ref.asset_id)}`,{params:{version:ref.version}}),
 submit:(body:unknown)=>withIdempotency('sandbox:submit',body,(key)=>http.post<{id:string;state:string}>('/sandbox/runs',body,{headers:{'Idempotency-Key':key}})),
 list:(cursor='')=>http.get<{items:SandboxRun[];next_cursor:string}>('/sandbox/runs',{params:{cursor}}),
 get:(id:string)=>http.get<SandboxRun>(`/sandbox/runs/${encodeURIComponent(id)}`),
 cancel:(run:SandboxRun)=>http.post(`/sandbox/runs/${encodeURIComponent(run.id)}/cancel`,{id:run.id,tenant_id:run.tenant_id,user_id:run.user_id,request_sha256:run.request_sha256}),
}
