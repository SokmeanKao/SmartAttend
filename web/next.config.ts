import type { NextConfig } from "next";

// Browser calls same-origin /api/* so the session cookie is first-party.
// Next rewrites proxy to the Go API (Compose service name at image build time).
const apiProxyTarget =
  process.env.API_PROXY_TARGET?.trim() || "http://localhost:8080";

const nextConfig: NextConfig = {
  async rewrites() {
    return [
      {
        source: "/api/:path*",
        destination: `${apiProxyTarget}/api/:path*`,
      },
    ];
  },
};

export default nextConfig;
