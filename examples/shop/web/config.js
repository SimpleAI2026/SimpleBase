// 发布到对象存储站点时，把 apiBase 改成独立运行的 BFF 公开地址（如 http://127.0.0.1:8092）。
// 留空表示与页面同源（本地 `node server/main.mjs` 同时托管前端）。这里不要放任何 API Key。
window.__SB_CONFIG__ = { apiBase: '' }
