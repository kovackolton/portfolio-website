export type Project = { id: number; title: string; description: string; url: string; tags: string[] };
export async function api<T>(path: string, init?: RequestInit): Promise<T> {
    const res = await fetch("/api" + path, { ...init, headers: { "Content-Type": "application/json", ...init?.headers }, cache: "no-store" });
    if (!res.ok) { 
        const body = await res.json().catch(() => ({})); 
        throw new Error(body.error || "Request failed (" + res.status + ")"); 
    }
    return res.status === 204 ? undefined as T : res.json();
}
