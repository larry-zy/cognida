import {http} from '@/utils/request'
import {withIdempotency} from '@/utils/idempotent'
import type {RefID} from '@/api/lifecycle'
export interface HFSearchItem{repo_id:string;sha?:string;downloads:number;likes:number;private:boolean;gated:boolean}
export interface HFDataset{repo_id:string;revision:string;card:string;license:string;files:{path:string;size:number|null;format:string;supported:boolean}[];configs:{name:string;splits:{name:string;files:string[]}[]}[];warnings:string[]}
export interface HFColumnMapping{source:string;target:string;encoding?:'json'}
export interface HFRequest{repo_id:string;revision:string;config:string;split:string;files:string[];field_mapping:{text_field:string;id_field?:string}|{columns:HFColumnMapping[]};max_rows:number;max_bytes:number;credential_id?:string}
export interface HFManifest{repo_id:string;revision:string;config:string;split:string;files:{path:string;sha256:string;bytes:number;rows:number;total_rows:number|null;truncated:boolean}[];field_mapping:HFRequest['field_mapping'];license:string;rows:number;bytes:number;total_rows:number|null;sampled:boolean;truncated:boolean;excluded_columns:string[];acquired_at:string;request_hash:string}
export interface HFPreview{number_encoding?:"exact_string";manifest:HFManifest;columns:string[];column_types:Record<string,string>;source_columns:{name:string;type:string}[];rows:unknown[][];preview_rows:number}
export interface HFImport{id:string;status:string;request:HFRequest;manifest?:HFManifest;output_refs:RefID[];error_code?:string;deadline:string}
export const huggingfaceApi={
 capabilities:()=>http.get<{items:{id:string;decoder_ready:boolean;fixed_revision_preview:boolean;static_file_import:boolean}[]}>('/data-connectors'),
 search:(q:string,cursor='',credential_id?:string)=>http.get<{items:HFSearchItem[];next_cursor:string}>('/huggingface/datasets',{params:{q,cursor,limit:20,credential_id}}),
 dataset:(repo_id:string,revision:string,credential_id?:string)=>http.get<HFDataset>('/huggingface/dataset',{params:{repo_id,revision,credential_id}}),
 preview:(data:HFRequest)=>http.post<HFPreview>('/huggingface/preview',data,{timeout:120000}),
 import:(data:HFRequest)=>withIdempotency('/huggingface/imports',data,key=>http.post<HFImport>('/huggingface/imports',data,{headers:{'Idempotency-Key':key},timeout:120000})),
 get:(id:string)=>http.get<HFImport>(`/huggingface/imports/${encodeURIComponent(id)}`),
}
