// src/errors.ts
var SimpleBaseError = class extends Error {
  constructor(opts) {
    super(opts.message);
    this.name = "SimpleBaseError";
    this.status = opts.status;
    this.code = opts.code;
    this.requestId = opts.requestId;
    this.details = opts.details;
  }
};
function errorFromResponse(status, body, requestId) {
  const b = body && typeof body === "object" ? body : {};
  const errObj = b.error && typeof b.error === "object" ? b.error : b;
  const code = String(errObj.code || b.code || "http_error");
  const message = String(errObj.message || b.message || `HTTP ${status}`);
  const rid = requestId || (typeof b.request_id === "string" ? b.request_id : void 0);
  return new SimpleBaseError({ message, status, code, requestId: rid, details: body });
}

// src/http.ts
function joinUrl(base, path) {
  const b = base.replace(/\/+$/, "");
  const p = path.startsWith("/") ? path : `/${path}`;
  return `${b}${p}`;
}
function buildQuery(query) {
  if (!query) return "";
  const sp = new URLSearchParams();
  for (const [k, v] of Object.entries(query)) {
    if (v === void 0 || v === null) continue;
    sp.set(k, String(v));
  }
  const s = sp.toString();
  return s ? `?${s}` : "";
}
function createHttpClient(opts) {
  const baseUrl = opts.url.replace(/\/+$/, "");
  const f = opts.fetch ?? fetch;
  return {
    baseUrl,
    projectId: opts.projectId,
    async request(method, path, req = {}) {
      const url = joinUrl(baseUrl, path) + buildQuery(req.query);
      const headers = {
        Authorization: `Bearer ${opts.apiKey}`,
        Accept: "application/json",
        ...opts.headers || {},
        ...req.headers || {}
      };
      let body;
      if (req.formData) {
        body = req.formData;
      } else if (req.body !== void 0) {
        headers["Content-Type"] = "application/json";
        body = JSON.stringify(req.body);
      }
      let res;
      try {
        res = await f(url, { method, headers, body });
      } catch (e) {
        throw new SimpleBaseError({
          message: e instanceof Error ? e.message : "network error",
          status: 0,
          code: "network_error"
        });
      }
      const requestId = res.headers.get("x-request-id") || void 0;
      const text = await res.text();
      let parsed = void 0;
      if (text) {
        try {
          parsed = JSON.parse(text);
        } catch {
          parsed = { message: text };
        }
      }
      if (!res.ok) {
        throw errorFromResponse(res.status, parsed, requestId);
      }
      if (res.status === 204 || text === "") {
        return void 0;
      }
      return parsed;
    }
  };
}
function projectPath(projectId, suffix) {
  const s = suffix.startsWith("/") ? suffix : `/${suffix}`;
  return `/v1/projects/${encodeURIComponent(projectId)}${s}`;
}

// src/databases.ts
function createDatabasesApi(http) {
  const base = (s) => projectPath(http.projectId, s);
  return {
    list(opts) {
      return http.request("GET", base("/databases"), {
        query: { limit: opts?.limit, cursor: opts?.cursor }
      });
    },
    get(databaseId) {
      return http.request("GET", base(`/databases/${encodeURIComponent(databaseId)}`));
    },
    create(input) {
      return http.request("POST", base("/databases"), { body: input });
    },
    remove(databaseId) {
      return http.request("DELETE", base(`/databases/${encodeURIComponent(databaseId)}`));
    }
  };
}

// src/sql.ts
function createSqlApi(http, databaseId) {
  const db = encodeURIComponent(databaseId);
  const p = (suffix) => projectPath(http.projectId, `/databases/${db}${suffix}`);
  return {
    query(sql, args = [], opts) {
      if (!sql?.trim()) throw new Error("sql is required");
      return http.request("POST", p("/query"), {
        body: { sql, args, max_rows: opts?.maxRows }
      });
    },
    execute(sql, args = []) {
      if (!sql?.trim()) throw new Error("sql is required");
      return http.request("POST", p("/execute"), { body: { sql, args } });
    },
    batch(statements, opts) {
      return http.request("POST", p("/batch"), {
        body: {
          statements: statements.map((s) => ({ sql: s.sql, args: s.args ?? [] })),
          transactional: opts?.transactional ?? true
        }
      });
    }
  };
}

// src/collections.ts
var NAME_RE = /^[A-Za-z_][A-Za-z0-9_]*$/;
function assertCollectionName(name) {
  if (!NAME_RE.test(name)) {
    throw new Error(`invalid collection name: ${name}`);
  }
}
function createCollectionsApi(http, databaseId) {
  const db = encodeURIComponent(databaseId);
  const root = () => projectPath(http.projectId, `/databases/${db}/data/collections`);
  function collection(name) {
    assertCollectionName(name);
    const col = encodeURIComponent(name);
    return {
      list() {
        return http.request("GET", `${root()}/${col}`);
      },
      insert(doc) {
        return http.request("POST", `${root()}/${col}/documents`, { body: doc });
      },
      update(id, doc) {
        return http.request(
          "PUT",
          `${root()}/${col}/documents/${encodeURIComponent(id)}`,
          { body: doc }
        );
      },
      remove(id) {
        return http.request(
          "DELETE",
          `${root()}/${col}/documents/${encodeURIComponent(id)}`
        );
      }
    };
  }
  return {
    list() {
      return http.request("GET", root());
    },
    async create(name) {
      assertCollectionName(name);
      await http.request("POST", root(), { body: { name } });
    },
    collection
  };
}

// src/storage.ts
function toBlob(body, contentType) {
  if (typeof Blob !== "undefined" && body instanceof Blob) return body;
  if (typeof body === "string") {
    return new Blob([body], { type: contentType || "text/plain" });
  }
  if (body instanceof ArrayBuffer) {
    return new Blob([body], { type: contentType || "application/octet-stream" });
  }
  if (ArrayBuffer.isView(body)) {
    const view = body;
    const copy = new Uint8Array(view.byteLength);
    copy.set(new Uint8Array(view.buffer, view.byteOffset, view.byteLength));
    return new Blob([copy], { type: contentType || "application/octet-stream" });
  }
  return new Blob([body], { type: contentType });
}
function createStorageApi(http) {
  const objects = () => projectPath(http.projectId, "/s3/objects");
  const presignPath = () => projectPath(http.projectId, "/s3/presign");
  return {
    list(opts) {
      return http.request("GET", objects(), {
        query: {
          prefix: opts?.prefix,
          refresh: opts?.refresh ? 1 : void 0
        }
      });
    },
    upload(key, body, opts) {
      const fd = new FormData();
      fd.set("key", key);
      const blob = toBlob(body, opts?.contentType);
      const filename = opts?.filename || key.split("/").pop() || "file";
      fd.set("file", blob, filename);
      return http.request("POST", objects(), { formData: fd });
    },
    remove(key) {
      return http.request("DELETE", objects(), { query: { key } });
    },
    presign(key) {
      return http.request("GET", presignPath(), { query: { key } });
    }
  };
}

// src/client.ts
function requireDbId(id, action) {
  if (!id) {
    throw new SimpleBaseError({
      message: `${action} requires databaseId \u2014 pass createClient({ databaseId }) or use client.database(id)`,
      status: 0,
      code: "database_id_required"
    });
  }
  return id;
}
function createClient(opts) {
  if (!opts?.url) throw new Error("url is required");
  if (!opts?.apiKey) throw new Error("apiKey is required");
  if (!opts?.projectId) throw new Error("projectId is required");
  const http = createHttpClient(opts);
  const databases = createDatabasesApi(http);
  const storage = createStorageApi(http);
  let defaultDatabaseId = opts.databaseId;
  const database = (id) => {
    const sql = createSqlApi(http, id);
    const collections = createCollectionsApi(http, id);
    return {
      id,
      sql,
      collections,
      collection: (name) => collections.collection(name)
    };
  };
  const client = {
    projectId: opts.projectId,
    url: http.baseUrl,
    databases,
    storage,
    get sql() {
      return createSqlApi(http, requireDbId(defaultDatabaseId, "sql"));
    },
    collection(name) {
      return createCollectionsApi(http, requireDbId(defaultDatabaseId, "collection")).collection(name);
    },
    database,
    raw: {
      request: http.request.bind(http)
    }
  };
  return client;
}
export {
  SimpleBaseError,
  createClient
};
//# sourceMappingURL=index.js.map