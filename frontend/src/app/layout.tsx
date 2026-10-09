import type { Metadata } from "next";
import "./globals.css";
export const metadata: Metadata = { title: "Portfolio | Engineering in the open", description: "Projects, engineering notes, and live service health." };
export default function RootLayout({ children }: { children: React.ReactNode }) { return <html lang="en"><body>{children}</body></html>; }
