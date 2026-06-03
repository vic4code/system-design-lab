"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useAuth } from "@/context/AuthContext";

function LogOutIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" className="w-4 h-4">
      <path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4" />
      <polyline points="16 17 21 12 16 7" />
      <line x1="21" y1="12" x2="9" y2="12" />
    </svg>
  );
}

function HomeIcon({ filled }: { filled?: boolean }) {
  return filled ? (
    <svg viewBox="0 0 24 24" fill="currentColor" className="w-6 h-6">
      <path d="M13.5 1.515a3 3 0 0 0-3 0L3 5.845a2 2 0 0 0-1 1.732V21a1 1 0 0 0 1 1h6v-5h4v5h6a1 1 0 0 0 1-1V7.577a2 2 0 0 0-1-1.732l-7.5-4.33z" />
    </svg>
  ) : (
    <svg viewBox="0 0 24 24" fill="currentColor" className="w-6 h-6">
      <path d="M12.5 3.247a1 1 0 0 0-1 0L4 7.577V20h4.5v-6h3v6H16V7.577l-3.5-4.33zM13.5 1.515a3 3 0 0 0-3 0L3 5.845a2 2 0 0 0-1 1.732V21a1 1 0 0 0 1 1h6v-6h4v6h6a1 1 0 0 0 1-1V7.577a2 2 0 0 0-1-1.732l-7.5-4.33z" />
    </svg>
  );
}

function SearchIcon({ filled }: { filled?: boolean }) {
  return filled ? (
    <svg viewBox="0 0 24 24" fill="currentColor" className="w-6 h-6">
      <path d="M10.533 1.279c-5.18 0-9.407 4.927-8.917 10.237.41 4.404 3.986 7.98 8.39 8.39a9.196 9.196 0 0 0 5.817-1.533l3.332 3.332a1 1 0 1 0 1.414-1.414l-3.332-3.332a9.196 9.196 0 0 0 1.533-5.817c-.41-4.404-3.986-7.98-8.39-8.39a9.2 9.2 0 0 0-.847-.473z" />
    </svg>
  ) : (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" className="w-6 h-6">
      <circle cx="11" cy="11" r="8" />
      <path d="m21 21-4.35-4.35" />
    </svg>
  );
}

function UploadIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" className="w-6 h-6">
      <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" />
      <polyline points="17 8 12 3 7 8" />
      <line x1="12" y1="3" x2="12" y2="15" />
    </svg>
  );
}

function LibraryIcon({ filled }: { filled?: boolean }) {
  return filled ? (
    <svg viewBox="0 0 24 24" fill="currentColor" className="w-6 h-6">
      <path d="M3 22a1 1 0 0 1-1-1V3a1 1 0 0 1 2 0v18a1 1 0 0 1-1 1zM15.5 2.134A1 1 0 0 0 14 3v18a1 1 0 0 0 1 1h6a1 1 0 0 0 1-1V6.464a1 1 0 0 0-.5-.866l-6-3.464zM9 2a1 1 0 0 0-1 1v18a1 1 0 1 0 2 0V3a1 1 0 0 0-1-1z" />
    </svg>
  ) : (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" className="w-6 h-6">
      <path d="M2 3h6a4 4 0 0 1 4 4v14a3 3 0 0 0-3-3H2z" />
      <path d="M22 3h-6a4 4 0 0 0-4 4v14a3 3 0 0 1 3-3h7z" />
    </svg>
  );
}

function HeartIcon({ filled }: { filled?: boolean }) {
  return filled ? (
    <svg viewBox="0 0 24 24" fill="currentColor" className="w-6 h-6">
      <path d="M12 21.35l-1.45-1.32C5.4 15.36 2 12.28 2 8.5 2 5.42 4.42 3 7.5 3c1.74 0 3.41.81 4.5 2.09C13.09 3.81 14.76 3 16.5 3 19.58 3 22 5.42 22 8.5c0 3.78-3.4 6.86-8.55 11.54L12 21.35z" />
    </svg>
  ) : (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" className="w-6 h-6">
      <path d="M20.84 4.61a5.5 5.5 0 0 0-7.78 0L12 5.67l-1.06-1.06a5.5 5.5 0 0 0-7.78 7.78l1.06 1.06L12 21.23l7.78-7.78 1.06-1.06a5.5 5.5 0 0 0 0-7.78z" />
    </svg>
  );
}

const navItems = [
  { href: "/",          label: "Home",      Icon: HomeIcon },
  { href: "/search",    label: "Search",    Icon: SearchIcon },
  { href: "/favorites", label: "Favorites", Icon: HeartIcon },
  { href: "/upload",    label: "Upload",    Icon: UploadIcon },
  { href: "/playlists", label: "Library",   Icon: LibraryIcon },
];

export default function Sidebar() {
  const path = usePathname();
  const { state, user, logout } = useAuth();

  return (
    <aside className="w-60 shrink-0 bg-black flex flex-col py-6 px-3 gap-2 overflow-y-auto">
      {/* Logo */}
      <Link href="/" className="flex items-center gap-2 px-3 mb-4">
        <svg viewBox="0 0 167 167" fill="none" className="w-8 h-8">
          <circle cx="83.5" cy="83.5" r="83.5" fill="#1DB954" />
          <path
            d="M122 107c-1.6 0-2.6-.5-4-1.3-11.5-6.8-25.7-10.4-40.8-10.4-8.4 0-16.9 1-24.8 3-1.3.4-3 .9-4.3.9-3.5 0-5.8-2.7-5.8-5.8 0-3.7 2.3-5.7 5.1-6.4 9.6-2.5 19.7-3.8 29.9-3.8 17.4 0 33.8 4.2 47 12 2.3 1.4 3.6 2.9 3.6 5.9 0 3.2-2.5 5.9-5.9 5.9zM133 82c-2 0-3.2-.8-4.7-1.7-13.5-8-31.3-12.7-51.2-12.7-9.8 0-18.9 1.3-26.4 3.4-1.7.5-2.6.9-4.2.9-4.2 0-7.4-3.3-7.4-7.5 0-4.3 2.7-7 5.8-7.9 9.4-2.8 19.9-4.3 32.4-4.3 22 0 42.6 5.4 58.9 15.3 2.8 1.6 4.4 3.7 4.4 7.4 0 4.2-3.3 7.1-7.6 7.1zM145 53c-2.2 0-3.5-.6-5.2-1.6C123 40.9 101 35.5 77 35.5c-11 0-22.3 1.4-32 4.1-1.5.4-3.3 1-5.3 1-5.1 0-9-4-9-9 0-5.3 3.3-8.3 6.7-9.3 11.4-3.3 24-5.1 39.7-5.1 27 0 52.4 6.5 72 18.3 3 1.8 5 4.1 5 8.7-.1 5.1-4.1 8.8-9.1 8.8z"
            fill="black"
          />
        </svg>
        <span className="text-white font-bold text-base tracking-tight">Beatstream</span>
      </Link>

      {/* Main nav */}
      <nav className="flex flex-col gap-1">
        {navItems.map(({ href, label, Icon }) => {
          const active = href === "/" ? path === "/" : path.startsWith(href);
          return (
            <Link
              key={href}
              href={href}
              className={`flex items-center gap-4 px-3 py-2 rounded text-sm font-semibold transition-colors ${
                active ? "text-white" : "text-muted hover:text-white"
              }`}
            >
              <Icon filled={active} />
              {label}
            </Link>
          );
        })}
      </nav>

      {/* Bottom: user section */}
      <div className="mt-auto pt-4 border-t border-white/10">
        {state.status === "loading" ? null : state.status === "authenticated" ? (
          <div className="flex items-center gap-3 px-3 py-2 rounded hover:bg-surface transition-colors group">
            {/* Avatar */}
            <div className="w-8 h-8 rounded-full bg-accent flex items-center justify-center text-black text-sm font-bold shrink-0 select-none">
              {user!.name[0].toUpperCase()}
            </div>
            <p className="flex-1 text-sm font-semibold truncate">{user!.name}</p>
            <button
              onClick={logout}
              className="opacity-0 group-hover:opacity-100 transition-opacity text-muted hover:text-white"
              title="Log out"
            >
              <LogOutIcon />
            </button>
          </div>
        ) : (
          <div className="flex flex-col gap-2 px-3">
            <Link href="/login" className="btn-primary text-center py-2 text-sm">
              Log in
            </Link>
            <Link
              href="/register"
              className="text-center py-2 text-sm text-muted hover:text-white transition-colors"
            >
              Sign up
            </Link>
          </div>
        )}
      </div>
    </aside>
  );
}
