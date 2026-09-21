import { test } from "node:test";
import assert from "node:assert/strict";
import { readFile, mkdir } from "node:fs/promises";
import { build } from "esbuild";
import React from "react";
import TestRenderer, { act } from "react-test-renderer";

const source = await readFile(new URL("../src/App.jsx", import.meta.url), "utf8");
const cache = new URL("../node_modules/.cache/recommendation-test/", import.meta.url);
await mkdir(cache, { recursive: true });
await build({stdin:{contents:source+"\nexport { FeedPage, fetchFeedPage, appendFeedItems, mapFeedItem, requiresAuthFeed, createFeedRequestID, normalizeRoute };",loader:"jsx",resolveDir:new URL("../",import.meta.url).pathname.replace(/^\/(\w:)/,"$1")},bundle:true,platform:"node",format:"esm",external:["react"],jsx:"automatic",outfile:new URL("app.mjs",cache).pathname.replace(/^\/(\w:)/,"$1")});
const app = await import(new URL("app.mjs",cache));
globalThis.window = {addEventListener(){},removeEventListener(){},clearTimeout, setTimeout};

// flush 等待请求链和组件副作用完成。
async function flush(){await act(async()=>{await new Promise(resolve=>setTimeout(resolve,10));});}
// response 创建模拟 JSON 响应。
function response(body,status=200){return {ok:status<400,status,json:async()=>body};}
// item 创建可供页面渲染的测试视频。
function item(id){return {video_id:id,author_id:10,title:`video ${id}`,media_url:"test.mp4",cover_url:"test.jpg"};}
// deferred 创建可手动完成的响应以验证过期请求隔离。
function deferred(){let resolve;const promise=new Promise(done=>{resolve=done;});return {promise,resolve};}

test("请求路径、上下文、卡片标识及分页去重",async()=>{
 const calls=[];globalThis.fetch=async(path,options)=>{calls.push({path,options});return response({items:[]});};
 await app.fetchFeedPage("recommend","token","cursor","req");
 assert.equal(calls[0].path,"/api/feed/query");assert.equal(calls[0].options.method,"POST");
 assert.deepEqual(JSON.parse(calls[0].options.body),{scene:"recommend",cursor:"cursor",limit:10,context:{request_id:"req"}});
 await app.fetchFeedPage("hot","token","a+/=","req");assert.equal(new URL(calls[1].path,"http://test").searchParams.get("cursor"),"a+/=");
 assert.deepEqual(app.appendFeedItems([item(1)],[item(1),item(2),item(2)]).map(v=>v.video_id),[1,2]);
 assert.equal(app.mapFeedItem(item(1),"recommend","req").request_id,"req");
 assert.equal(app.normalizeRoute("/recommend"),"/recommend");assert.equal(app.requiresAuthFeed("recommend"),true);assert.equal(app.requiresAuthFeed("timeline"),false);
 assert.ok(app.createFeedRequestID("recommend").length<=64);
});

test("匿名推荐不请求数据并显示登录入口",async()=>{
 globalThis.fetch=async()=>{throw Error("匿名请求不应发出");};let root;
 await act(async()=>{root=TestRenderer.create(React.createElement(app.FeedPage,{feedScene:"recommend",session:{token:""},onNavigate(){}}));});
 assert.match(JSON.stringify(root.toJSON()),/登录后查看推荐流/);await act(async()=>root.unmount());
});

test("分页串行、重复视频去重、曝光仅一次及旧响应隔离",async()=>{
 const pending=deferred();const calls=[];const session={token:"token",clearAuth(){throw Error("旧请求不得清理会话");}};
 globalThis.fetch=async(path,options={})=>{calls.push({path,options});if(path.startsWith("/api/playback-config"))return response({preload_count:3,buffer_ms:1200});if(path.startsWith("/api/preload-videos"))return response({items:[]});if(path.includes("following"))return response({items:[]});if(path==="/api/video-view-events")return response({});if(path==="/api/feed/query"){const body=JSON.parse(options.body);return body.cursor?pending.promise:response({items:[item(1),item(2),item(3)],has_more:true,next_cursor:"next"});}return response({items:[item(90)],has_more:false});};
 let root;const navigate=()=>{};await act(async()=>{root=TestRenderer.create(React.createElement(app.FeedPage,{feedScene:"recommend",session,onNavigate:navigate}));});await flush();
 assert.equal(calls.filter(c=>c.path==="/api/feed/query").length,2);
 assert.equal(calls.filter(c=>c.path==="/api/video-view-events").length,1);
 await act(async()=>root.update(React.createElement(app.FeedPage,{feedScene:"timeline",session,onNavigate:navigate})));await flush();
 await act(async()=>pending.resolve(response({items:[item(999)],has_more:false})));await flush();
 const tree=JSON.stringify(root.toJSON());assert.match(tree,/video 90/);assert.doesNotMatch(tree,/video 999/);assert.match(tree,/已到末尾/);
 await act(async()=>root.unmount());
});

test("续页保留请求标识并拒绝重复卡片",async()=>{
 const calls=[];let page=0;
 globalThis.fetch=async(path,options={})=>{calls.push({path,options});if(path.startsWith("/api/playback-config"))return response({preload_count:3,buffer_ms:1200});if(path.startsWith("/api/preload-videos"))return response({items:[]});if(path.includes("following"))return response({items:[]});if(path==="/api/video-view-events")return response({});page++;if(page===1)return response({items:[item(1)],has_more:true,next_cursor:"next"});return response({items:[item(1),item(2)],has_more:false});};
 let root;await act(async()=>{root=TestRenderer.create(React.createElement(app.FeedPage,{feedScene:"recommend",session:{token:"token",clearAuth(){}},onNavigate(){}}));});await flush();
 const queries=calls.filter(c=>c.path==="/api/feed/query").map(c=>JSON.parse(c.options.body));assert.equal(queries.length,2);assert.equal(queries[0].context.request_id,queries[1].context.request_id);assert.equal(calls.filter(c=>c.path==="/api/video-view-events").length,1);
 await act(async()=>root.unmount());
});


test("续页失败保留当前卡片且不循环重试，认证失效退出登录",async()=>{
 for(const status of [500,401]){
  let queries=0,cleared=0,navigated="";
  globalThis.fetch=async(path)=>{if(path.startsWith("/api/playback-config"))return response({preload_count:3,buffer_ms:1200});if(path.startsWith("/api/preload-videos"))return response({items:[]});if(path.includes("following"))return response({items:[]});if(path==="/api/video-view-events")return response({});queries++;return queries===1?response({items:[item(1)],has_more:true,next_cursor:"next"}):response({error:"failed"},status);};
  let root;await act(async()=>{root=TestRenderer.create(React.createElement(app.FeedPage,{feedScene:"recommend",session:{token:"token",clearAuth(){cleared++;}},onNavigate(path){navigated=path;}}));});await flush();await flush();
  assert.equal(queries,2);assert.match(JSON.stringify(root.toJSON()),/video 1/);
  assert.equal(cleared,status===401?1:0);assert.equal(navigated,status===401?"/auth":"");
  await act(async()=>root.unmount());
 }
});
