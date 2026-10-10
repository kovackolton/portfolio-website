"use client";
import { useEffect, useState } from "react";
import { ArrowUpRight, Activity, Terminal } from "lucide-react";
import { Button } from "@/components/ui/button";
import { api, type Project } from "@/lib/api";
type Health = { status: string; uptime_seconds: number; database_ms: number; database_connected: boolean; redis_connected: boolean; memory_mb: number; requests: number; cache_hits: number; cache_misses: number };
export default function Home() {
	const [projects, setProjects] = useState<Project[]>([]);
	const [projectError, setProjectError] = useState("");
	const [health, setHealth] = useState<Health | null>(null);
	const [offline, setOffline] = useState(false);
	const [message, setMessage] = useState("");
	const [busy, setBusy] = useState(false);
	const [stackSlide, setStackSlide] = useState(0);

	useEffect(() => {
		api<Project[]>("/projects").then(setProjects).catch(e => setProjectError(e.message));
		let active = true;
		const poll = async () => { 
			try { 
				const res = await fetch("/api/health", { cache: "no-store" }); 
				const data = await res.json(); 
				if (!data.status) throw new Error(); 
				if (active) { 
					setHealth(data); 
					setOffline(false); 
				} 
			} 
			catch { 
				if (active) setOffline(true); 
			} 
		};
		void poll(); 
		const timer = setInterval(poll, 10000); 
		return () => { 
			active = false; clearInterval(timer); 
		};
	}, []);
	async function contact(e: React.FormEvent<HTMLFormElement>) {
		e.preventDefault(); setBusy(true); setMessage("");
		const form = e.currentTarget; const data = Object.fromEntries(new FormData(form));
		try { 
			const result = await api<{ message: string }>("/contact", { method: "POST", body: JSON.stringify(data) }); 
			setMessage(result.message); 
			form.reset(); 
		} 
		catch (e) { 
			setMessage((e as Error).message); 
		} 
		finally { 
			setBusy(false); 
		}
	}
	const metric = (value: string) => offline ? "Unavailable" : health ? value : "Connecting…";
	
	return (
		<main className="mx-auto max-w-6xl px-6">
			<header className="flex items-center justify-between border-b border-zinc-800 py-7">
				<a href="/" className="flex items-center gap-2 font-semibold">
					<Terminal size={20} className="text-primary" /> Kolton Kovac<span className="text-zinc-500">/</span>
				</a>
				<nav className="flex gap-5 text-sm text-zinc-400">
					<a href="#work">Work</a>
					<a href="#systems">Systems</a>
					<a href="#contact">Contact</a>
				</nav>
			</header>

			<section className="grid gap-12 py-24 md:grid-cols-[1.5fr_1fr]">
				<div>
					<p className="mb-6 font-mono text-xs uppercase tracking-[.2em] text-primary">Kolton Kovac · Software engineer</p>
					<h1 className="text-5xl font-semibold leading-[1.08] tracking-tight md:text-7xl">Building software.<br /><span className="text-zinc-500">Solving real problems.</span></h1>
					<p className="mt-7 max-w-lg text-lg leading-relaxed text-zinc-400">Full-stack software engineer with 6+ years of experience building applications, cloud infrastructure, and backend services. I enjoy tackling complex problems, exploring new technologies, and turning ideas into reliable, practical software. A home for the things I build, the problems I solve, and the systems behind them.</p>
					<div className="mt-9 flex gap-3">
						<Button asChild>
							<a href="#work">Explore projects <ArrowUpRight size={16} /></a></Button><Button variant="outline" asChild>
							<a href="#contact">Get in touch</a></Button>
					</div>
				</div>
				
				<div role="region" aria-roledescription="carousel" aria-label="Website stack and skills" className="self-center rounded-xl border border-zinc-800 bg-zinc-900/60 p-7 font-mono text-sm">
					<div className="mb-8 flex gap-2" role="group" aria-label="Choose a slide">
						{["Website stack", "My skills"].map((label, index) => (
							<button
								key={label}
								type="button"
								aria-label={label}
								aria-pressed={stackSlide === index}
								aria-controls="stack-slides"
								onClick={() => setStackSlide(index)}
								className="flex size-6 cursor-pointer items-center justify-center rounded-full focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-primary"
							>
								<span className={"size-2 rounded-full transition-colors " + (stackSlide === index ? "bg-primary" : "bg-zinc-600 hover:bg-zinc-400")} />
							</button>
						))}
					</div>
					<div id="stack-slides" className="grid" aria-live="polite">
					<div role="group" aria-roledescription="slide" aria-label="1 of 2: Website stack" aria-hidden={stackSlide !== 0} className={"col-start-1 row-start-1 " + (stackSlide === 0 ? "visible" : "invisible")}>
					<p className="text-zinc-500">// built from the ground up</p>
					<p className="mt-5"><span className="text-primary">const</span> stack = {"{"}</p>
					<div className="space-y-3 py-4 pl-5 text-zinc-400">
						<p>interface: <span className="text-white">"Next.js + React"</span>,</p>
						<p>engine: <span className="text-white">"Go + Gin"</span>,</p>
						<p>storage: <span className="text-white">"PostgreSQL"</span>,</p>
						<p>cache: <span className="text-white">"Redis"</span></p>
					</div>
					<p>{"};"}</p>
					</div>
					<div role="group" aria-roledescription="slide" aria-label="2 of 2: My skills" aria-hidden={stackSlide !== 1} className={"col-start-1 row-start-1 " + (stackSlide === 1 ? "visible" : "invisible")}>
						<p className="text-zinc-500">// my skills</p>
						<p className="mt-5"><span className="text-primary">const</span> skills = {"{"}</p>
						<div className="space-y-3 py-4 pl-5 text-zinc-400">
							{/* Add your skills here using <p> rows, like the website stack above. */}
						</div>
						<p>{"};"}</p>
					</div>
					</div>
				</div>
			</section>

			<section id="work" className="border-t border-zinc-800 py-16">
				<div className="mb-9 flex items-end justify-between">
					<div>
						<p className="font-mono text-xs text-primary">01 / SELECTED WORK</p>
						<h2 className="mt-3 text-3xl font-semibold">Projects with a purpose.</h2>
					</div>
					<span className="text-sm text-zinc-500">{projects.length} projects</span>
				</div>
				{projectError ? <p className="rounded-xl border border-zinc-800 p-8 text-zinc-400">{projectError}. Start the backend to load projects.</p> : projects.length === 0 ? <div className="rounded-xl border border-dashed border-zinc-700 p-10 text-zinc-400">Projects will appear here as they are published.</div> : <div className="grid gap-5 md:grid-cols-2">{projects.map((p,i) => <article key={p.id} className="rounded-xl border border-zinc-800 bg-zinc-900/40 p-7"><p className="mb-10 font-mono text-xs text-zinc-500">PROJECT / {String(i+1).padStart(2,"0")}</p><h3 className="text-2xl">{p.title}</h3><p className="my-4 whitespace-pre-wrap leading-relaxed text-zinc-400">{p.description}</p><div className="mb-7 flex flex-wrap gap-2">{p.tags.map((t,j)=><span key={j} className="rounded bg-zinc-800 px-2 py-1 font-mono text-xs text-zinc-300">{t}</span>)}</div>{p.url && <a href={p.url} target="_blank" rel="noopener noreferrer" className="inline-flex gap-2 text-sm text-primary">View project <ArrowUpRight size={16}/></a>}</article>)}</div>}
			</section>

			<section id="systems" className="border-t border-zinc-800 py-16">
				<div className="mb-9 flex flex-wrap items-end justify-between gap-4">
					<div>
						<p className="font-mono text-xs text-primary">02 / UNDER THE HOOD</p>
						<h2 className="mt-3 text-3xl font-semibold">A portfolio that shows its work.</h2>
					</div>
					<span className="flex items-center gap-2 text-sm text-zinc-400">
						<Activity size={16} className={offline ? "text-amber-300" : "text-primary"} />
						{offline ? "Backend offline" : health?.status || "Connecting"} · refreshes every 10s
					</span>
				</div>
				<div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
					{[["Process uptime",metric(Math.floor((health?.uptime_seconds || 0)/60) + " min")],["Database round trip",metric((health?.database_ms.toFixed(2) || "") + " ms")],["Go heap allocated",metric((health?.memory_mb.toFixed(1) || "") + " MB")],["Project cache hits",metric((health?.cache_hits || 0) + " / " + ((health?.cache_hits || 0)+(health?.cache_misses || 0)))]].map(([label,value])=><div key={label} className="rounded-xl border border-zinc-800 p-6"><p className="text-xs text-zinc-500">{label}</p><p className="mt-4 font-mono text-2xl">{value}</p></div>)}
				</div>
				<p className="mt-5 text-sm text-zinc-500">Measured by the running API. Process uptime resets on restart; it is not a historical availability percentage. Redis: {offline ? "unknown" : health ? health.redis_connected ? "connected" : "disconnected" : "connecting"}.</p>
			</section>

			<section id="contact" className="grid gap-12 border-t border-zinc-800 py-16 md:grid-cols-2">
				<div>
					<p className="font-mono text-xs text-primary">03 / SAY HELLO</p>
					<h2 className="mt-3 text-4xl font-semibold">Let’s build something<br />worth sharing.</h2>
					<p className="mt-5 text-zinc-400">Send a note about a project or an opportunity. Messages are saved privately in PostgreSQL.</p>
				</div>
				<form onSubmit={contact} className="space-y-4">
					<div>
						<label htmlFor="name">Name</label>
						<input id="name" name="name" required maxLength={120} autoComplete="name" />
					</div>
					<div>
						<label htmlFor="email">Email</label>
						<input id="email" name="email" type="email" required maxLength={254} autoComplete="email" />
					</div>
					<div>
						<label htmlFor="message">Message</label>
						<textarea id="message" name="message" rows={4} required maxLength={4000}/>
					</div>
					<Button disabled={busy}>{busy ? "Sending…" : "Send message"}<ArrowUpRight size={16}/></Button>
					<p role="status" className="text-sm text-zinc-400">{message}</p>
				</form>
			</section>

			<footer className="flex justify-between border-t border-zinc-800 py-8 text-xs text-zinc-500">
				<span>Built with Go, Next.js, and a little curiosity.</span>
				<span>Portfolio / {new Date().getFullYear()}</span>
			</footer>
		</main>
	);
}
