import { test } from "node:test";
import assert from "node:assert/strict";
import { readFile, mkdir } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import { build } from "esbuild";
import React from "react";
import TestRenderer, { act } from "react-test-renderer";

const source = await readFile(new URL("../src/App.jsx", import.meta.url), "utf8");
const cache = new URL("../node_modules/.cache/message-test/", import.meta.url);
await mkdir(cache, { recursive: true });
await build({ stdin: { contents: source + "\nexport { App, AppShell, TopNav, LoginPage, MessagesPage, PageMessage, fetchMessages, markMessagesRead, appendMessages, messageIcon, formatBadgeCount, normalizeRoute };", loader: "jsx", resolveDir: fileURLToPath(new URL("../", import.meta.url)) }, bundle: true, platform: "node", format: "esm", external: ["react"], jsx: "automatic", outfile: fileURLToPath(new URL("app.mjs", cache)) });
const app = await import(new URL("app.mjs", cache));

// response 构造模拟接口响应，覆盖成功和带错误正文的状态码。
function response(body, status = 200) { return { ok: status < 400, status, json: async () => body }; }
// deferred 创建可手动完成的响应，验证重复操作和过期响应隔离。
function deferred() { let resolve; const promise = new Promise(done => { resolve = done; }); return { promise, resolve }; }
// message 构造覆盖消息类型和已读状态的测试消息。
function message(id, is_read = false, type = "LIKE") { return { id, user_id: 42, type, title: `通知 ${id}`, content: `内容 ${id}`, is_read, created_at: new Date().toISOString() }; }
// flush 等待组件副作用和请求链执行完毕。
async function flush() { await act(async () => { await new Promise(resolve => setTimeout(resolve, 0)); }); }
// text 返回渲染树文本，供状态和错误展示断言。
function text(root) { return JSON.stringify(root.toJSON()); }
// button 查找包含指定标签文本的按钮。
function button(root, label) { return root.root.findAllByType("button").find(node => JSON.stringify(node.children.filter(child => typeof child === "string")).includes(label)); }
// rows 查找消息项按钮，避免将容器或导航算作消息。
function rows(root) { return root.root.findAllByType("button").filter(node => node.props.className?.startsWith("message-item ")); }
// session 创建可统计认证失效次数的会话夹具。
function session(token = "token") { return { token, user: { id: 42 }, cleared: 0, clearAuth() { this.cleared++; } }; }
// mount 创建消息页并等待首次请求，返回用于交互断言的组件树。
async function mount(props) { let root; await act(async () => { root = TestRenderer.create(React.createElement(app.MessagesPage, props)); }); await flush(); return root; }
// browser 初始化应用路由和本地会话，模拟历史导航与浏览器事件。
function browser(path = "/messages", authenticated = true) {
 const store = new Map(authenticated ? [["videofeed.accessToken", "token"], ["videofeed.user", JSON.stringify({ id: 42, nickname: "测试用户" })]] : []);
 globalThis.localStorage = { getItem: key => store.get(key) || null, setItem: (key, value) => store.set(key, value), removeItem: key => store.delete(key) };
 globalThis.window = { location: { pathname: path }, history: { pushState(_state, _title, next) { window.location.pathname = next; } }, addEventListener() {}, removeEventListener() {}, setTimeout, clearTimeout };
}

test("消息请求、游标编码、全部已读和所有图标角标规则", async () => {
 const calls = []; globalThis.fetch = async (path, options) => { calls.push({ path, options }); return response({}); };
 await app.fetchMessages("token", "a+/="); const url = new URL(calls[0].path, "http://test");
 assert.equal(url.pathname, "/api/messages"); assert.equal(url.searchParams.get("limit"), "20"); assert.equal(url.searchParams.get("cursor"), "a+/="); assert.equal(calls[0].options.headers.Authorization, "Bearer token");
 await app.markMessagesRead("token", [1, 2]); await app.markMessagesRead("token");
 assert.equal(calls[1].options.method, "PATCH"); assert.deepEqual(JSON.parse(calls[1].options.body), { message_ids: [1, 2] }); assert.deepEqual(JSON.parse(calls[2].options.body), { message_ids: [] });
 assert.deepEqual(app.appendMessages([message(1)], [message(1, true), message(2), message(2)]).map(item => [item.id, item.is_read]), [[1, false], [2, false]]);
 assert.deepEqual(["like", "COMMENT", "FOLLOW", "SYSTEM", "unknown"].map(app.messageIcon), ["favorite", "chat_bubble", "person_add", "campaign", "notifications"]);
 assert.deepEqual([0, -1, NaN, Infinity, 1, "99", 100].map(app.formatBadgeCount), ["", "", "", "", "1", "99", "99+"]);
 assert.equal(app.normalizeRoute("/messages"), "/messages"); assert.equal(app.normalizeRoute("/feed"), "/timeline");
});

test("加载、错误重试和空列表状态", async () => {
 const pending = deferred(); let calls = 0;
 globalThis.fetch = async () => ++calls === 1 ? pending.promise : response({ items: [] });
 const root = await mount({ session: session(), onNavigate() {}, onUnreadChange() {} });
 assert.match(text(root), /正在加载消息/);
 await act(async () => pending.resolve(response({ error: "暂时不可用" }, 500))); await flush();
 assert.match(text(root), /暂时不可用/); await act(async () => button(root, "重试").props.onClick()); await flush();
 assert.match(text(root), /暂无消息/); assert.equal(calls, 2); await act(async () => root.unmount());
});

test("分页去重、并发续页拦截、失败保留列表并支持重试", async () => {
 const pending = deferred(); let calls = 0;
 globalThis.fetch = async () => { calls++; if (calls === 1) return response({ items: [message(1)], has_more: true, next_cursor: "next" }); if (calls === 2) return pending.promise; return response({ items: [message(1), message(2), message(2)], has_more: false }); };
 const root = await mount({ session: session(), onNavigate() {}, onUnreadChange() {} }); const load = button(root, "加载更多").props.onClick;
 await act(async () => { load(); load(); }); assert.equal(calls, 2);
 await act(async () => pending.resolve(response({ error: "续页失败" }, 500))); await flush(); assert.equal(rows(root).length, 1); assert.match(text(root), /续页失败/);
 await act(async () => button(root, "加载更多").props.onClick()); await flush(); assert.equal(rows(root).length, 2); assert.equal(button(root, "加载更多"), undefined); await act(async () => root.unmount());
});

test("单条与全部已读串行、失败不乐观修改、重试后刷新角标", async () => {
 const pending = deferred(); const writes = []; let refreshed = 0;
 globalThis.fetch = async (_path, options = {}) => {
  if (options.method !== "PATCH") return response({ items: [message(1), message(2)], has_more: false });
  writes.push(JSON.parse(options.body)); return writes.length === 1 ? pending.promise : response({ updated_count: 1 });
 };
 const root = await mount({ session: session(), onNavigate() {}, onUnreadChange() { refreshed++; } });
 const read = rows(root)[0].props.onClick; const all = button(root, "全部已读").props.onClick;
 await act(async () => { read(); read(); all(); }); assert.equal(writes.length, 1);
 await act(async () => pending.resolve(response({ error: "写入失败" }, 500))); await flush(); assert.match(rows(root)[0].props.className, /unread/); assert.equal(refreshed, 0);
 await act(async () => rows(root)[0].props.onClick()); assert.match(rows(root)[0].props.className, / read/); assert.equal(refreshed, 1);
 await act(async () => rows(root)[0].props.onClick()); assert.equal(writes.length, 2);
 await act(async () => button(root, "全部已读").props.onClick()); assert.deepEqual(writes, [{ message_ids: [1] }, { message_ids: [1] }, { message_ids: [] }]); assert.equal(refreshed, 2); assert.ok(rows(root).every(row => row.props.className.endsWith(" read")));
 await act(async () => root.unmount());
});

test("访客不查询，读取及两种已读操作的401均清理会话并导航登录", async () => {
 for (const mode of ["anonymous", "load", "single", "all"]) {
  let calls = 0, path = ""; const auth = session(mode === "anonymous" ? "" : "token");
  globalThis.fetch = async (_path, options = {}) => { calls++; return mode === "load" || options.method === "PATCH" ? response({ error: "expired" }, 401) : response({ items: [message(1)] }); };
  const root = await mount({ session: auth, onNavigate(next) { path = next; }, onUnreadChange() {} });
  if (mode === "single") await act(async () => rows(root)[0].props.onClick());
  if (mode === "all") await act(async () => button(root, "全部已读").props.onClick());
  assert.equal(path, "/auth"); assert.equal(auth.cleared, mode === "anonymous" ? 0 : 1); if (mode === "anonymous") assert.equal(calls, 0);
  await act(async () => root.unmount());
 }
});

test("更换会话和卸载后忽略旧列表及已读错误，不清理新会话", async () => {
 for (const operation of ["load", "mark", "all"]) {
  const pending = deferred(); const first = session("old"), second = session("new"); let navigated = 0, refreshed = 0;
  globalThis.fetch = async (_path, options) => {
   if (options.headers.Authorization === "Bearer old" && (operation === "load" || options.method === "PATCH")) return pending.promise;
   return response({ items: [message(options.headers.Authorization === "Bearer new" ? 99 : 1)] });
  };
  const onNavigate = () => { navigated++; }, onUnreadChange = () => { refreshed++; };
  const root = await mount({ session: first, onNavigate, onUnreadChange });
  if (operation !== "load") await act(async () => { (operation === "mark" ? rows(root)[0] : button(root, "全部已读")).props.onClick(); });
  await act(async () => root.update(React.createElement(app.MessagesPage, { session: second, onNavigate, onUnreadChange }))); await flush();
  await act(async () => pending.resolve(response({ error: "old expired" }, 401))); await flush();
  assert.match(text(root), /通知 99/); assert.doesNotMatch(text(root), /old expired/); assert.equal(first.cleared + second.cleared + navigated + refreshed, 0);
  await act(async () => root.unmount());
 }
 const pending = deferred(); let effects = 0; globalThis.fetch = async () => pending.promise;
 const root = await mount({ session: { token: "old", clearAuth() { effects++; } }, onNavigate() { effects++; }, onUnreadChange() {} });
 await act(async () => root.unmount()); await act(async () => pending.resolve(response({}, 401))); await flush(); assert.equal(effects, 0);
});

test("角标更新不会重载消息页，桌面和侧栏入口均可用", async () => {
 browser(); let loads = 0, stats = 0;
 globalThis.fetch = async path => path === "/api/message-stats/unread" ? (stats++, response({ unread_count: 120 })) : (loads++, response({ items: [message(1)] }));
 let root; await act(async () => { root = TestRenderer.create(React.createElement(app.App)); }); await flush(); await flush();
 assert.equal(loads, 1); assert.equal(stats, 1); assert.equal(root.root.findAllByType("span").filter(node => node.props.className?.includes("nav-badge")).length, 2); assert.match(text(root), /99\+/);
 await act(async () => root.root.findByProps({ "aria-label": "通知" }).props.onClick()); await flush(); assert.equal(loads, 1);
 await act(async () => root.unmount());
 const navigated = []; await act(async () => { root = TestRenderer.create(React.createElement(app.AppShell, { authenticated: false, route: "/timeline", unreadCount: 10, onNavigate(path) { navigated.push(path); } })); });
 await act(async () => { root.root.findByProps({ "aria-label": "通知" }).props.onClick(); root.root.findByProps({ "aria-label": "消息" }).props.onClick(); });
 assert.deepEqual(navigated, ["/auth", "/auth"]); assert.equal(root.root.findAllByType("span").filter(node => node.props.className?.includes("nav-badge")).length, 0); await act(async () => root.unmount());
});

test("角标的旧账号响应不能覆盖新账号，错误401清零而普通错误保留", async () => {
 browser(); const pending = deferred(); let mode = "pending";
 globalThis.fetch = async (path, options) => {
  if (path !== "/api/message-stats/unread") return response({ items: [] });
  if (options.headers.Authorization === "Bearer token") return pending.promise;
  return mode === "401" ? response({}, 401) : mode === "500" ? response({}, 500) : response({ unread_count: 7 });
 };
 let root; await act(async () => { root = TestRenderer.create(React.createElement(app.App)); }); await flush();
 const first = root.root.findByType(app.MessagesPage).props.session;
 await act(async () => first.setAuth("new", { id: 77 })); await flush();
 await act(async () => pending.resolve(response({ unread_count: 99 }))); await flush();
 assert.equal(root.root.findByType(app.AppShell).props.unreadCount, 7);
 mode = "500"; await act(async () => root.root.findByType(app.MessagesPage).props.onUnreadChange()); assert.equal(root.root.findByType(app.AppShell).props.unreadCount, 7);
 mode = "401"; await act(async () => root.root.findByType(app.MessagesPage).props.onUnreadChange()); assert.equal(root.root.findByType(app.AppShell).props.unreadCount, 0);
 await act(async () => root.unmount());
});
