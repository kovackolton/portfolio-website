import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

const env = Object.fromEntries((await readFile(new URL("../.env", import.meta.url), "utf8")).trim().split(/\r?\n/).map(line => {
  const i = line.indexOf("=");
  return [line.slice(0, i), line.slice(i + 1)];
}));
const base = env.PUBLIC_ORIGIN || "http://localhost:3000";
assert(["localhost", "127.0.0.1"].includes(new URL(base).hostname), "Run this check only against the local development stack");
let cookie = "";
const ids = [];
const payload = sort_order => ({ title: "Temporary sort order check", description: "Removed after validation", url: "", tags: [], sort_order });
async function request(path, method = "GET", body) {
  return fetch(base + "/api" + path, {
    method, headers: { Origin: base, "Content-Type": "application/json", ...(cookie ? { Cookie: cookie } : {}) },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
}
async function list() {
  const response = await request("/projects");
  assert.equal(response.status, 200);
  return response.json();
}
try {
  const login = await request("/auth/login", "POST", { email: env.ADMIN_EMAIL, password: env.ADMIN_PASSWORD });
  assert.equal(login.status, 200);
  cookie = login.headers.get("set-cookie").split(";")[0];
  const before = (await list()).map(p => p.id);
  for (const order of [20, 10]) {
    const response = await request("/admin/projects", "POST", payload(order));
    assert.equal(response.status, 200);
    const project = await response.json();
    ids.push(project.id);
    assert.equal(project.sort_order, order);
  }
  let projects = await list();
  assert(projects.findIndex(p => p.id === ids[1]) < projects.findIndex(p => p.id === ids[0]), "Lower sort order must appear first");
  assert.equal((await request("/projects")).headers.get("x-cache"), "HIT");
  assert.equal((await request("/admin/projects/" + ids[0], "PUT", payload(5))).status, 200);
  projects = await list();
  assert.equal(projects.find(p => p.id === ids[0]).sort_order, 5);
  assert(projects.findIndex(p => p.id === ids[0]) < projects.findIndex(p => p.id === ids[1]), "Editing order must invalidate cached ordering");
  for (const invalid of [-1, 1.5, 2147483648]) {
    assert.equal((await request("/admin/projects/" + ids[0], "PUT", payload(invalid))).status, 400);
  }
  assert.equal((await request("/admin/projects/" + ids[0], "PUT", payload(10))).status, 200);
  projects = await list();
  assert(projects.findIndex(p => p.id === ids[1]) < projects.findIndex(p => p.id === ids[0]), "Ties must retain newest-first ordering");
  assert.deepEqual(projects.filter(p => !ids.includes(p.id)).map(p => p.id), before, "Existing project order must remain unchanged");
  console.log("PASS: create/edit sort order, cached reorder, equal-order ties, invalid values, and existing project order.");
} finally {
  for (const id of ids) {
    const response = await request("/admin/projects/" + id, "DELETE");
    assert.equal(response.status, 204, "Temporary project cleanup failed");
  }
  if (cookie) await request("/auth/logout", "POST");
}
