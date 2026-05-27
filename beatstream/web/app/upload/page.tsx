"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";
import { useAuth } from "@/context/AuthContext";
import UploadForm from "@/components/UploadForm";

export default function UploadPage() {
  const { state } = useAuth();
  const router = useRouter();

  useEffect(() => {
    if (state.status === "guest") {
      router.push("/login");
    }
  }, [state.status, router]);

  if (state.status === "loading") {
    return (
      <div
        className="min-h-full"
        style={{ background: "linear-gradient(to bottom, #2a2a1a 0%, #121212 40%)" }}
      />
    );
  }

  if (state.status === "guest") {
    return null; // redirecting
  }

  return (
    <div
      className="min-h-full"
      style={{ background: "linear-gradient(to bottom, #2a2a1a 0%, #121212 40%)" }}
    >
      <div className="px-6 pt-14 pb-6 max-w-lg">
        <h1 className="text-3xl font-bold mb-8">Upload a Track</h1>
        <UploadForm />
      </div>
    </div>
  );
}
