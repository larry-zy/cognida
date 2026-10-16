import {http} from '@/utils/request'
import {withIdempotency} from '@/utils/idempotent'
import type {Page} from '@/api/governance'
export interface SFTInput {name:string;source_ref_id:string;standard_version_id:string;format:'qa'|'messages';columns:{question?:string;reference_answer?:string;messages?:string;tools?:string;split?:string;group?:string}}
export interface SFTDataset extends SFTInput {id:string;version:number;status:string;row_count:number;split_counts:Record<string,number>;content_digest:string;latest_version_id?:string;review_comment?:string;reviewed_by?:number}
export interface SFTVersion extends SFTDataset {dataset_id:string;dataset_revision:number}
export interface SFTRow {row_id:string;source_row_index:number;messages:Record<string,unknown>[];tools?:Record<string,unknown>[];split:string;group?:string}
export interface SFTExecution {id:string;status:string;domain_kind:string;domain_id?:string;error?:string;output_refs?:import('@/api/lifecycle').RefID[]}
const post=<T>(path:string,data:unknown)=>withIdempotency(path,data,key=>http.post<T>(path,data,{headers:{'Idempotency-Key':key}}))
export const sftApi={
 list:(cursor='')=>http.get<Page<SFTDataset>>('/sft-datasets',{params:{cursor,page_size:50}}),
 get:(id:string)=>http.get<SFTDataset>(`/sft-datasets/${encodeURIComponent(id)}`),
 execution:(id:string)=>http.get<SFTExecution>(`/governance-executions/${encodeURIComponent(id)}`),
 create:(data:SFTInput)=>post<SFTDataset>('/sft-datasets',data),
 rows:(id:string,cursor='')=>http.get<Page<SFTRow>>(`/sft-datasets/${encodeURIComponent(id)}/rows`,{params:{cursor,page_size:50}}),
 review:(id:string,expected_version:number,decision:'approved'|'rejected',comment:string)=>post<SFTDataset>(`/sft-datasets/${encodeURIComponent(id)}/review`,{expected_version,decision,comment}),
 publish:(id:string,expected_version:number)=>post<SFTVersion>(`/sft-datasets/${encodeURIComponent(id)}/publish`,{expected_version}),
 versions:(id:string,cursor='')=>http.get<Page<SFTVersion>>(`/sft-datasets/${encodeURIComponent(id)}/versions`,{params:{cursor,page_size:50}}),
 version:(id:string)=>http.get<SFTVersion>(`/sft-dataset-versions/${encodeURIComponent(id)}`),
}
