import { test } from "node:test";
import assert from "node:assert/strict";
import { mkdir, readFile } from "node:fs/promises";
import { build } from "esbuild";
import React from "react";
import TestRenderer, { act } from "react-test-renderer";

const source = await readFile(new URL("../src/App.jsx", import.meta.url), "utf8");
const cache = new URL("../node_modules/.cache/upload-test/", import.meta.url);
await mkdir(cache, { recursive: true });
await build({
    stdin: {
        contents: `${source}\nexport { UploadPage };`,
        loader: "jsx",
        resolveDir: new URL("../", import.meta.url).pathname.replace(/^\/(\w:)/, "$1"),
    },
    bundle: true,
    platform: "node",
    format: "esm",
    external: ["react"],
    jsx: "automatic",
    outfile: new URL("app.mjs", cache).pathname.replace(/^\/(\w:)/, "$1"),
});
const { UploadPage } = await import(new URL("app.mjs", cache));
URL.createObjectURL = () => "blob:test-cover";
URL.revokeObjectURL = () => {};

// renderUploadForm 准备已填写标题并选择视频、封面的发布表单。
async function renderUploadForm(onNavigate = () => {}) {
    let root;
    await act(async () => {
        root = TestRenderer.create(React.createElement(UploadPage, {
            session: { token: "test-token" },
            onNavigate,
        }));
    });
    const inputs = root.root.findAllByType("input");
    await act(async () => {
        inputs.find((input) => input.props.placeholder === "输入视频标题").props.onChange({
            target: { value: "test video" },
        });
        inputs.find((input) => input.props.accept === "video/*").props.onChange({
            target: { files: [new File(["video"], "test.mp4", { type: "video/mp4" })] },
        });
        inputs.find((input) => input.props.accept === "image/*").props.onChange({
            target: { files: [new File(["cover"], "cover.png", { type: "image/png" })] },
        });
    });
    return root;
}

test("组合发布失败时只发送一次请求并显示错误", async () => {
    const calls = [];
    globalThis.fetch = async (path, options) => {
        calls.push({path, options});
        return { ok: false, status: 500, json: async () => ({ error: "video tool is unavailable" }) };
    };
    const root = await renderUploadForm();
    await act(async () => {
        await root.root.findByType("form").props.onSubmit({ preventDefault() {} });
    });
    assert.equal(calls.length, 1);
    assert.equal(calls[0].path, "/api/videos/upload");
    assert.equal(calls[0].options.body.get("title"), "test video");
    assert.equal(calls[0].options.body.get("video").name, "test.mp4");
    assert.equal(calls[0].options.body.get("cover").name, "cover.png");
    assert.match(JSON.stringify(root.toJSON()), /video tool is unavailable/);
    await act(async () => root.unmount());
});

test("组合发布成功后进入个人资料", async () => {
    const calls = [];
    let navigated = "";
    globalThis.fetch = async (path, options) => {
        calls.push({path, options});
        return { ok: true, status: 201, json: async () => ({ id: 1 }) };
    };
    const root = await renderUploadForm((path) => { navigated = path; });
    await act(async () => {
        await root.root.findByType("form").props.onSubmit({ preventDefault() {} });
    });
    assert.equal(calls.length, 1);
    assert.equal(calls[0].path, "/api/videos/upload");
    assert.equal(calls[0].options.headers.Authorization, "Bearer test-token");
    assert.match(calls[0].options.headers["Idempotency-Key"], /^web-upload-/);
    assert.equal(navigated, "/profile");
    await act(async () => root.unmount());
});

test("同一内容失败重试复用幂等键", async () => {
    const keys = [];
    globalThis.fetch = async (_path, options) => {
        keys.push(options.headers["Idempotency-Key"]);
        return keys.length === 1
            ? { ok: false, status: 500, json: async () => ({ error: "temporary failure" }) }
            : { ok: true, status: 201, json: async () => ({ id: 1 }) };
    };
    const root = await renderUploadForm();
    for (let attempt = 0; attempt < 2; attempt++) {
        await act(async () => {
            await root.root.findByType("form").props.onSubmit({ preventDefault() {} });
        });
    }
    assert.equal(keys.length, 2);
    assert.equal(keys[0], keys[1]);
    await act(async () => root.unmount());
});
