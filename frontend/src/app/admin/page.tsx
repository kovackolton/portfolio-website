"use client";
import { useEffect, useState } from "react";
import { Button } from "@/components/ui/button";
import { api, type Project } from "@/lib/api";
export default function Admin() {
    const [signedIn,setSignedIn]=useState(false), [checking,setChecking]=useState(true), [projects,setProjects]=useState<Project[]>([]), [editing,setEditing]=useState<Project|null>(null), [error,setError]=useState(""), [busy,setBusy]=useState(false);
    const refresh=()=>api<Project[]>("/projects").then(setProjects);
    useEffect(()=>{ api("/auth/me").then(()=>{setSignedIn(true); return refresh();}).catch(()=>{}).finally(()=>setChecking(false)); },[]);
    async function run(action:()=>Promise<void>) { setBusy(true); setError(""); try { await action(); } catch(e){setError((e as Error).message);} finally{setBusy(false);} }
    function login(e:React.FormEvent<HTMLFormElement>) { e.preventDefault(); const data=Object.fromEntries(new FormData(e.currentTarget)); void run(async()=>{await api("/auth/login",{method:"POST",body:JSON.stringify(data)}); setSignedIn(true); await refresh();}); }
    function save(e:React.FormEvent<HTMLFormElement>) { e.preventDefault(); const form=e.currentTarget; const data=Object.fromEntries(new FormData(form)); void run(async()=>{await api(editing ? "/admin/projects/"+editing.id : "/admin/projects",{method:editing?"PUT":"POST",body:JSON.stringify({...data,tags:String(data.tags).split(",").map(t=>t.trim()).filter(Boolean)})}); setEditing(null); form.reset(); await refresh();}); }
    return (
        <main className="mx-auto max-w-3xl px-6 py-12">
            <a href="/" className="text-sm text-primary">← Portfolio</a>
            <h1 className="my-8 text-3xl font-semibold">Project administration</h1>
            { checking ? 
            <p>Checking session…</p> : 
                !signedIn ? 
                <form onSubmit={login} className="max-w-md space-y-5">
                    <div>
                        <label htmlFor="email">Admin email</label>
                        <input id="email" name="email" type="email" required autoComplete="username"/>
                    </div>
                    <div>
                        <label htmlFor="password">Password</label>
                        <input id="password" name="password" type="password" required maxLength={72} autoComplete="current-password"/>
                    </div>
                    <Button disabled={busy}>Sign in</Button>
                </form> 

                : 
                <>
                <Button variant="outline" disabled={busy} onClick={()=>void run(async()=>{await api("/auth/logout",{method:"POST"});setSignedIn(false);setProjects([]);})}>Sign out</Button>
                <form key={editing?.id || "new"} onSubmit={save} className="my-8 space-y-4 rounded-xl border border-zinc-800 p-6">
                    <h2 className="text-xl">{editing?"Edit project":"Add project"}</h2>
                    <div>
                        <label htmlFor="title">Title</label>
                        <input id="title" name="title" defaultValue={editing?.title} required maxLength={160}/>
                    </div>
                    <div>
                        <label htmlFor="description">Description</label>
                        <textarea id="description" name="description" defaultValue={editing?.description} required maxLength={4000} rows={4}/>
                    </div>
                    <div>
                        <label htmlFor="url">Project URL (optional)</label>
                        <input id="url" name="url" type="url" defaultValue={editing?.url} maxLength={2048}/>
                    </div>
                    <div>
                        <label htmlFor="tags">Tags (comma separated)</label>
                        <input id="tags" name="tags" defaultValue={editing?.tags.join(", ")}/>
                    </div>
                    <div className="flex gap-3">
                        <Button disabled={busy}>Save project</Button>
                        {
                        editing && <Button type="button" variant="outline" onClick={()=>setEditing(null)}>Cancel</Button>
                        }
                    </div>
                </form>
                <div className="space-y-4">
                    {
                    projects.map(p=>
                        <div key={p.id} className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-zinc-800 p-4">
                            <span>{p.title}</span>
                            <div className="flex gap-2">
                                <Button size="sm" variant="outline" disabled={busy} onClick={()=>setEditing(p)}>Edit</Button>
                                <Button size="sm" variant="outline" disabled={busy} onClick={()=>{if(window.confirm('Delete "'+p.title+'"?')) void run(async()=>{await api("/admin/projects/"+p.id,{method:"DELETE"});if(editing?.id===p.id)setEditing(null);await refresh();});}}>Delete</Button>
                            </div></div>
                            )
                    }
                </div>
            </>
            }
            <p role="alert" className="mt-5 text-amber-300">{error}</p>
        </main>
    );
}
