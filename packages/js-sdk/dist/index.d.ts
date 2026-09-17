type Json = string | number | boolean | null | {
    [key: string]: Json | undefined;
} | Json[];
interface CreateClientOptions {
    /** API origin, e.g. http://127.0.0.1:8080 */
    url: string;
    /** Bearer API key */
    apiKey: string;
    /** Project UUID */
    projectId: string;
    /** Default database id for sql/collection helpers */
    databaseId?: string;
    /** Inject custom fetch (tests / polyfill) */
    fetch?: typeof fetch;
    /** Extra headers on every request */
    headers?: Record<string, string>;
}
interface DatabaseInfo {
    id: string;
    name?: string;
    status?: string;
    kind?: string;
    [key: string]: unknown;
}
/** SQL query response — rows are positional arrays matching columns. */
interface QueryResult {
    columns: string[];
    rows: unknown[][];
    row_count: number;
    duration_ms?: number;
    request_id?: string;
    [key: string]: unknown;
}
interface ExecuteResult {
    rows_affected: number;
    last_insert_id?: number | null;
    durability?: string;
    duration_ms?: number;
    request_id?: string;
    [key: string]: unknown;
}
interface BatchResultItem {
    index: number;
    rows_affected?: number;
    last_insert_id?: number | null;
    duration_ms?: number;
    error_code?: string;
    error_message?: string;
}
interface BatchResult {
    results: BatchResultItem[];
    durability?: string;
    duration_ms?: number;
    request_id?: string;
    error?: {
        failed_index: number;
        code: string;
        message: string;
    };
    [key: string]: unknown;
}
interface BatchStatement {
    sql: string;
    args?: unknown[];
}
interface S3ObjectMeta {
    key: string;
    size?: number;
    lastModified?: string;
    etag?: string;
    content_type?: string;
    [key: string]: unknown;
}

type HttpMethod = 'GET' | 'POST' | 'PUT' | 'DELETE' | 'PATCH';
interface HttpClient {
    readonly baseUrl: string;
    readonly projectId: string;
    request<T = unknown>(method: HttpMethod, path: string, opts?: {
        query?: Record<string, string | number | boolean | undefined | null>;
        body?: unknown;
        formData?: FormData;
        headers?: Record<string, string>;
    }): Promise<T>;
}

interface DatabasesApi {
    list(opts?: {
        limit?: number;
        cursor?: string;
    }): Promise<{
        databases: DatabaseInfo[];
        next_cursor?: string;
    }>;
    get(databaseId: string): Promise<DatabaseInfo>;
    create(input: {
        name: string;
    }): Promise<DatabaseInfo>;
    open(databaseId: string): Promise<DatabaseInfo>;
    close(databaseId: string): Promise<void>;
    remove(databaseId: string): Promise<DatabaseInfo | void>;
}

interface SqlApi {
    query(sql: string, args?: unknown[], opts?: {
        maxRows?: number;
    }): Promise<QueryResult>;
    execute(sql: string, args?: unknown[]): Promise<ExecuteResult>;
    batch(statements: BatchStatement[], opts?: {
        transactional?: boolean;
    }): Promise<BatchResult>;
}

interface CollectionApi {
    list(): Promise<{
        rows: Record<string, unknown>[];
    }>;
    insert(doc: Record<string, Json | undefined>): Promise<Record<string, unknown>>;
    update(id: string, doc: Record<string, Json | undefined>): Promise<Record<string, unknown>>;
    remove(id: string): Promise<void>;
}
interface CollectionsApi {
    list(): Promise<{
        collections: string[];
    }>;
    create(name: string): Promise<void>;
    collection(name: string): CollectionApi;
}

type UploadBody = Blob | File | ArrayBuffer | Uint8Array | string;
interface StorageApi {
    list(opts?: {
        prefix?: string;
        refresh?: boolean;
    }): Promise<S3ObjectMeta[]>;
    upload(key: string, body: UploadBody, opts?: {
        contentType?: string;
        filename?: string;
    }): Promise<S3ObjectMeta>;
    remove(key: string): Promise<{
        ok?: boolean;
    }>;
    presign(key: string): Promise<{
        url: string;
    }>;
}

interface DatabaseRef {
    readonly id: string;
    sql: SqlApi;
    collections: CollectionsApi;
    collection(name: string): CollectionApi;
}
interface SimpleBaseClient {
    readonly projectId: string;
    readonly url: string;
    databases: DatabasesApi;
    storage: StorageApi;
    /** Default-database SQL helpers (requires databaseId in createClient or .database()) */
    sql: SqlApi;
    collection(name: string): CollectionApi;
    database(id: string): DatabaseRef;
    raw: {
        request: HttpClient['request'];
    };
}
declare function createClient(opts: CreateClientOptions): SimpleBaseClient;

declare class SimpleBaseError extends Error {
    readonly status: number;
    readonly code: string;
    readonly requestId?: string;
    readonly details?: unknown;
    constructor(opts: {
        message: string;
        status: number;
        code: string;
        requestId?: string;
        details?: unknown;
    });
}

export { type BatchResult, type BatchResultItem, type BatchStatement, type CollectionApi, type CollectionsApi, type CreateClientOptions, type DatabaseInfo, type DatabaseRef, type DatabasesApi, type ExecuteResult, type Json, type QueryResult, type S3ObjectMeta, type SimpleBaseClient, SimpleBaseError, type SqlApi, type StorageApi, type UploadBody, createClient };
