import { NextRequest } from "next/server";
export const dynamic = "force-dynamic";
async function proxy(req: NextRequest, { params }: { params: Promise<{ path: string[] }> }) {
    const { path } = await params;
    const headers = new Headers();
    for (const name of ["content-type", "cookie", "origin"]) { 
        const value = req.headers.get(name);
         if (value) headers.set(name, value); 
    }
    try {
        const res = await fetch(
            (process.env.API_URL || "http://localhost:8080") + "/api/" + path.map(encodeURIComponent).join("/") + req.nextUrl.search, 
            {
                method: req.method, 
                headers, 
                body: ["GET", "HEAD"].includes(req.method) ? undefined : await req.arrayBuffer(), 
                cache: "no-store", 
                signal: AbortSignal.timeout(10000),
            }
        );
        const output = new Headers({ "Cache-Control": "no-store" });
        for (const name of ["content-type", "set-cookie", "retry-after", "x-cache"]) { 
            const value = res.headers.get(name); 
            if (value) output.set(name, value); 
        }
        return new Response(res.body, { status: res.status, headers: output });
    } 
    catch { 
        return Response.json({ error: "Backend unavailable" }, { status: 503 }); 
    }
}
export { proxy as GET, proxy as POST, proxy as PUT, proxy as DELETE };
