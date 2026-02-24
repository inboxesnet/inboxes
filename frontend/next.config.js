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
    ];
  },
};

module.exports = nextConfig;
