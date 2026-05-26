"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";

const links = [
  { href: "/", label: "Tracks" },
  { href: "/search", label: "Search" },
  { href: "/upload", label: "Upload" },
  { href: "/playlists", label: "Playlists" },
];

export default function Navbar() {
  const path = usePathname();

  return (
    <nav className="bg-black px-6 py-4 flex items-center gap-8">
      <Link href="/" className="text-accent font-bold text-xl tracking-tight">
        Beatstream
      </Link>
      <div className="flex gap-6">
        {links.map(({ href, label }) => (
          <Link
            key={href}
            href={href}
            className={`text-sm font-medium transition-colors ${
              path === href ? "text-white" : "text-muted hover:text-white"
            }`}
          >
            {label}
          </Link>
        ))}
      </div>
    </nav>
  );
}
