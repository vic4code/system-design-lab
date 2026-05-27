"use client";

import { useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { register as apiRegister } from "@/lib/api";
import { useAuth } from "@/context/AuthContext";

export default function RegisterPage() {
  const { login } = useAuth();
  const router = useRouter();
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError("");
    setLoading(true);
    try {
      const { token, user } = await apiRegister(email, password, name);
      login(token, user);
      router.push("/");
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : "Registration failed");
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="min-h-full flex items-center justify-center bg-bg">
      <div className="w-full max-w-sm">
        <div className="text-center mb-8">
          <svg viewBox="0 0 167 167" fill="none" className="w-12 h-12 mx-auto mb-4">
            <circle cx="83.5" cy="83.5" r="83.5" fill="#1DB954" />
            <path d="M122 107c-1.6 0-2.6-.5-4-1.3-11.5-6.8-25.7-10.4-40.8-10.4-8.4 0-16.9 1-24.8 3-1.3.4-3 .9-4.3.9-3.5 0-5.8-2.7-5.8-5.8 0-3.7 2.3-5.7 5.1-6.4 9.6-2.5 19.7-3.8 29.9-3.8 17.4 0 33.8 4.2 47 12 2.3 1.4 3.6 2.9 3.6 5.9 0 3.2-2.5 5.9-5.9 5.9z" fill="black" />
          </svg>
          <h1 className="text-2xl font-bold">Sign up for Beatstream</h1>
        </div>

        <form onSubmit={handleSubmit} className="bg-surface rounded-lg p-8 space-y-4">
          <div>
            <label className="block text-sm font-semibold mb-2">What should we call you?</label>
            <input
              type="text"
              required
              autoFocus
              value={name}
              onChange={(e) => setName(e.target.value)}
              className="input"
              placeholder="Your name"
            />
          </div>
          <div>
            <label className="block text-sm font-semibold mb-2">Email address</label>
            <input
              type="email"
              required
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              className="input"
              placeholder="name@example.com"
            />
          </div>
          <div>
            <label className="block text-sm font-semibold mb-2">Password</label>
            <input
              type="password"
              required
              minLength={6}
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              className="input"
              placeholder="At least 6 characters"
            />
          </div>

          {error && <p className="text-sm text-red-400">{error}</p>}

          <button
            type="submit"
            disabled={loading}
            className="btn-primary w-full py-3 text-base mt-2"
          >
            {loading ? "Creating account…" : "Sign Up"}
          </button>
        </form>

        <p className="text-center text-sm text-muted mt-6">
          Already have an account?{" "}
          <Link href="/login" className="text-white underline hover:text-accent transition-colors">
            Log in
          </Link>
        </p>
      </div>
    </div>
  );
}
