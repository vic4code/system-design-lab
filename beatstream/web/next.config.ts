import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  // Proxy /v1/* to the backend so the frontend avoids CORS in local dev.
  // In production set NEXT_PUBLIC_API_URL to your deployed API URL instead.
  async rewrites() {
    const apiUrl = process.env.API_URL ?? "http://localhost";
    return [
      {
        source: "/v1/:path*",
        destination: `${apiUrl}/v1/:path*`,
      },
    ];
  },
};

export default nextConfig;
