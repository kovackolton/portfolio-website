import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
const env = Object.fromEntries((await readFile(new URL("../.env", import.meta.url), "utf8")).trim().split(/\r?\n/).map(line => { const i=line.indexOf("="); return [line.slice(0,i),line.slice(i+1)]; }));
const base = env.PUBLIC_ORIGIN || "http://localhost:3000";
let cookie = "", id;
async function request(path,method="GET",body,origin=base) {
 return fetch(base+"/api"+path,{method,headers:{"Content-Type":"application/json",Origin:origin,...(cookie?{Cookie:cookie}:{})},body:body===undefined?undefined:JSON.stringify(body)});
}
try {
 assert.equal((await fetch(base)).status,200);
 const health=await request("/health");assert.equal(health.status,200);
 assert.equal((await health.json()).redis_connected,true);
 assert.equal((await request("/admin/projects","POST",{title:"Unauthorized",description:"Test"})).status,401);
 assert.equal((await request("/auth/login","POST",{email:env.ADMIN_EMAIL,password:env.ADMIN_PASSWORD},"https://untrusted.example")).status,403);
 const login=await request("/auth/login","POST",{email:env.ADMIN_EMAIL,password:env.ADMIN_PASSWORD});assert.equal(login.status,200);
 cookie=login.headers.get("set-cookie").split(";")[0];
 assert.equal((await request("/auth/me")).status,200);
 const created=await request("/admin/projects","POST",{title:"Framework smoke test",description:"Temporary validation project",url:"https://example.com",tags:["test"]});assert.equal(created.status,200);id=(await created.json()).id;
 let list=await request("/projects");assert((await list.json()).some(p=>p.id===id));
 list=await request("/projects");assert.equal(list.headers.get("x-cache"),"HIT");
 assert.equal((await request("/admin/projects/"+id,"PUT",{title:"Updated smoke test",description:"Temporary validation project",url:"",tags:[]})).status,200);
 list=await request("/projects");assert.equal((await list.json()).find(p=>p.id===id).title,"Updated smoke test");
 assert.equal((await request("/admin/projects/"+id,"PUT",{title:"Unsafe URL",description:"Test",url:"javascript:alert(1)",tags:[]})).status,400);
 assert.equal((await request("/admin/projects/"+id,"DELETE")).status,204);
 list=await request("/projects");assert(!(await list.json()).some(p=>p.id===id));id=undefined;
 assert.equal((await request("/auth/logout","POST")).status,204);
 assert.equal((await request("/auth/me")).status,401);
 for(let i=0;i<5;i++) assert.equal((await request("/contact","POST",{name:"Test",email:"invalid",message:"Rate limiter test; not stored"})).status,400);
 assert.equal((await request("/contact","POST",{name:"Test",email:"invalid",message:"Not stored"})).status,429);
 console.log("PASS: pages, health, authorization, CSRF, login/logout, CRUD, cache invalidation, URL validation, and contact rate limit.");
} finally {
 if(id && cookie) await request("/admin/projects/"+id,"DELETE").catch(()=>{});
}
