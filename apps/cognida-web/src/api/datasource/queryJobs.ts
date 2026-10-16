import { http } from '@/utils/request'
export interface QueryInput { datasource_id: string; sql: string; max_rows: number; max_bytes: number; timeout_seconds: number; max_scan_bytes: number }
export interface QueryJob {id:string;input:QueryInput;engine:string;status:string;remote_status?:string;error?:string;row_count:number;bytes:number;truncated:boolean;cancel_requested:boolean;cleanup_pending:boolean;deadline:string}
export interface QueryResults {columns:string[];column_types:Record<string,string>;rows:Record<string,unknown>[];next_cursor?:string;truncated:boolean}
export const queryJobsApi = {
 list:(cursor='')=>http.get<{items:QueryJob[];next_cursor?:string}>('/query-jobs',{params:{cursor,page_size:20}}),
 get:(id:string)=>http.get<QueryJob>(`/query-jobs/${encodeURIComponent(id)}`),
 create:(input:QueryInput,key:string)=>http.post<QueryJob>('/query-jobs',input,{headers:{'Idempotency-Key':key}}),
 cancel:(id:string)=>http.post<QueryJob>(`/query-jobs/${encodeURIComponent(id)}/cancel`,{}),
 results:(id:string,cursor='')=>http.get<QueryResults>(`/query-jobs/${encodeURIComponent(id)}/results`,{params:{cursor,page_size:100}}),
}
