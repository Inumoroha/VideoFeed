import { test } from "node:test";
import assert from "node:assert/strict";
import { readFile, mkdir } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import { build } from "esbuild";
import React from "react";
import TestRenderer, { act } from "react-test-renderer";

const source = await readFile(new URL("../src/App.jsx", import.meta.url), "utf8");
const cache = new URL("../node_modules/.cache/playback-test/", import.meta.url);
await mkdir(cache, { recursive: true });
await build({ stdin: { contents: source + "\nexport { FeedPage, VideoStage, fetchPlaybackConfig, fetchPreloadVideos, normalizePlaybackConfig, detectNetworkType, prewarmVideoAssets, prewarmVideoMetadata, createVideoQoSState, buildPlaybackQoSPayload, createPlaybackQoSKey };", loader: "jsx", resolveDir: fileURLToPath(new URL("../", import.meta.url)) }, bundle: true, platform: "node", format: "esm", external: ["react"], jsx: "automatic", outfile: fileURLToPath(new URL("app.mjs", cache)) });
const app = await import(new URL("app.mjs", cache));

// network 注入可配置网络信息，避免测试依赖宿主浏览器。
function network(type) { Object.defineProperty(globalThis, "navigator", { configurable: true, value: { connection: { effectiveType: type } } }); }
// response 创建可供请求函数消费的成功或失败响应。
function response(body, status = 200) { return { ok: status < 400, status, json: async () => body }; }
// flush 等待组件副作用与网络回调完成。
async function flush() { await act(async () => { await new Promise(resolve => setTimeout(resolve, 0)); }); }

test("网络检测、配置回退及请求参数保持原提交行为", async () => {
 for (const [raw, expected] of [["wifi", "WiFi"], ["5g", "5G"], ["4g", "4G"], ["3g", "3G"], ["2g", "DEFAULT"]]) { network(raw); assert.equal(app.detectNetworkType(), expected); }
 assert.deepEqual(app.normalizePlaybackConfig({}), { platform: "Web", network_type: "DEFAULT", preload_count: 3, buffer_ms: 1200 });
 assert.equal(app.normalizePlaybackConfig({ preload_count: 99, buffer_ms: -1 }).preload_count, 10);
 assert.equal(app.normalizePlaybackConfig({ preload_count: 0 }).preload_count, 1);
 const calls = []; globalThis.fetch = async (path, options) => { calls.push({ path, options }); return response({}); };
 network("4g"); await app.fetchPlaybackConfig("token"); await app.fetchPreloadVideos("token", 9, 4);
 assert.equal(calls[0].path, "/api/playback-config?platform=Web&network_type=4G");
 assert.equal(calls[1].path, "/api/preload-videos?current_video_id=9&limit=4");
 assert.equal(calls[0].options.headers.Authorization, "Bearer token");
});

test("预加载去重，并在成功、错误与超时三条路径释放全部资源", () => {
 for (const event of ["loadedmetadata", "canplay", "error", "timeout"]) {
  const nodes = []; const images = []; const timers = new Map(); let nextTimer = 0;
  globalThis.Image = class { set src(value) { images.push(value); } };
  globalThis.window = { setTimeout(callback, ms) { assert.equal(ms, 4000); timers.set(++nextTimer, callback); return nextTimer; }, clearTimeout(id) { timers.delete(id); } };
  globalThis.document = { body: { appendChild(node) { nodes.push(node); } }, createElement(tag) { assert.equal(tag, "video"); return { style: {}, events: new Map(), loads: 0, removed: false, setAttribute() {}, removeAttribute(name) { delete this[name]; }, addEventListener(name, callback) { this.events.set(name, callback); }, removeEventListener(name) { this.events.delete(name); }, load() { this.loads++; }, remove() { this.removed = true; } }; } };
  const loaded = new Set(); const items = [{ media_url: "/one.mp4", cover_url: "/one.jpg" }];
  app.prewarmVideoAssets(items, loaded); app.prewarmVideoAssets(items, loaded);
  assert.equal(nodes.length, 1); assert.equal(images.length, 1); assert.equal(nodes[0].preload, "metadata");
  if (event === "timeout") [...timers.values()][0](); else nodes[0].events.get(event)();
  assert.equal(nodes[0].events.size, 0); assert.equal(timers.size, 0); assert.equal(nodes[0].src, undefined); assert.equal(nodes[0].removed, true); assert.equal(nodes[0].loads, 2);
 }
 delete globalThis.document;
});

test("质量指标规范化、空报告过滤与每个片段的稳定幂等键", () => {
 assert.equal(app.buildPlaybackQoSPayload({}), null);
 assert.deepEqual(app.buildPlaybackQoSPayload({ firstFrameMs: 3.7, stutterCount: 2, watchMs: 10.6 }), { first_frame_ms: 4, stutter_count: 2, watch_ms: 11 });
 assert.deepEqual(app.buildPlaybackQoSPayload({ watchMs: 10 }), { stutter_count: 0, watch_ms: 10 });
 const state = app.createVideoQoSState(9); assert.equal(state.videoID, 9); assert.equal(state.firstFrameMs, undefined);
 assert.equal(app.createPlaybackQoSKey({ video_id: 9 }, state), `web-qos-9-${state.reportID}`);
});

test("只有活动视频播放，暂停与卸载不重复报告，等待与首帧指标被提交", async () => {
 const reports = []; let plays = 0; let pauses = 0;
 const props = { item: { video_id: 1, media_url: "/video.mp4" }, active: true, onPlaybackQoS: (item, metrics) => reports.push({ item, metrics }) };
 const node = { play() { plays++; return Promise.resolve(); }, pause() { pauses++; } };
 let root; await act(async () => { root = TestRenderer.create(React.createElement(app.VideoStage, props), { createNodeMock: element => element.type === "video" ? node : null }); });
 let video = root.root.findByType("video"); assert.equal(video.props.preload, "metadata"); assert.equal(plays, 1);
 await act(async () => { video.props.onLoadedData(); video.props.onPlaying(); video.props.onWaiting(); video.props.onPause(); });
 assert.equal(reports.length, 1); assert.equal(reports[0].metrics.stutterCount, 1); assert.ok(reports[0].metrics.firstFrameMs >= 0);
 await act(async () => { video.props.onPause(); }); assert.equal(reports.length, 1);
 await act(async () => { root.update(React.createElement(app.VideoStage, { ...props, active: false })); });
 video = root.root.findByType("video"); assert.equal(video.props.preload, "none"); assert.equal(video.props.autoPlay, false); assert.ok(pauses > 0);
 await act(async () => { video.props.onPlaying(); video.props.onWaiting(); root.unmount(); }); assert.equal(reports.length, 1);
});

test("播放配置请求失败回退到三条预加载，登录失效触发认证导航", async () => {
 for (const status of [500, 401]) {
  network("4g"); const paths = []; const navigations = []; let cleared = 0;
  globalThis.window = { setTimeout, clearTimeout, addEventListener() {}, removeEventListener() {} };
  globalThis.fetch = async (path) => { paths.push(path); if (path.startsWith("/api/playback-config")) return response({ error: "failed" }, status); if (path.includes("following")) return response({ items: [] }); if (path.startsWith("/api/preload-videos")) return response({ items: [] }); if (path === "/api/video-view-events") return response({}); return response({ items: [{ video_id: 1, author_id: 2, media_url: "/video.mp4" }], has_more: false }); };
  let root; await act(async () => { root = TestRenderer.create(React.createElement(app.FeedPage, { feedScene: "timeline", session: { token: "token", user: { id: 42 }, clearAuth() { cleared++; } }, onNavigate(path) { navigations.push(path); } })); }); await flush();
  assert.ok(paths.some(path => path.startsWith("/api/playback-config")));
  if (status === 500) assert.ok(paths.some(path => path.includes("/api/preload-videos?current_video_id=1&limit=3"))); else { assert.ok(cleared > 0); assert.ok(navigations.includes("/auth")); }
  await act(async () => root.unmount());
 }
});
