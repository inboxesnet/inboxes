/** @type {import('next').NextConfig} */
const nextConfig = {
  reactStrictMode: true,
  output: "standalone",
  async rewrites() {
    // In Docker: defaults to http://backend:8080 (compose service name).
    // In local dev: set BACKEND_URL=http://localhost:8080.
    // These only fire when the browser uses relative paths (NEXT_PUBLIC_API_URL empty).
    // When NEXT_PUBLIC_API_URL is set, the browser hits the backend directly and
    // these rewrites are never triggered.
    const backend = process.env.BACKEND_URL || "http://backend:8080";
    return [
      {
        source: "/api/:path*",
        destination: `${backend}/api/:path*`,
      },
      // MCP endpoint and OAuth discovery metadata live on the backend but
      // outside /api — the MCP spec fixes their paths at the domain root.
      {
        source: "/mcp",
        destination: `${backend}/mcp`,
      },
      {
        source: "/.well-known/oauth-protected-resource",
        destination: `${backend}/.well-known/oauth-protected-resource`,
      },
      {
        source: "/.well-known/oauth-authorization-server",
        destination: `${backend}/.well-known/oauth-authorization-server`,
      },
    ];
  },
};

module.exports = nextConfig;
