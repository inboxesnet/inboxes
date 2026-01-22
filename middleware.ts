import { NextRequest, NextResponse } from "next/server";
import { jwtVerify } from "jose";

const SESSION_COOKIE = "session";

function getSecret() {
  const secret = process.env.SESSION_SECRET;
  if (!secret) throw new Error("SESSION_SECRET is not set");
  return new TextEncoder().encode(secret);
}

export async function middleware(request: NextRequest) {
  const token = request.cookies.get(SESSION_COOKIE)?.value;

  if (!token) {
    return unauthorized(request);
  }

  try {
    await jwtVerify(token, getSecret());
    return NextResponse.next();
  } catch {
    return unauthorized(request);
  }
}

function unauthorized(request: NextRequest) {
  // For API routes, return 401 JSON
  if (request.nextUrl.pathname.startsWith("/api/")) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }
  // For page routes, redirect to login
  const loginUrl = new URL("/login", request.url);
  return NextResponse.redirect(loginUrl);
}

export const config = {
  matcher: [
    // Protect all /api/* routes except /api/auth/*
    "/api/((?!auth/).*)",
    // Protect app pages (inbox, settings, etc.) but not auth pages or static
    "/inbox/:path*",
    "/sent/:path*",
    "/search/:path*",
    "/settings/:path*",
    "/dashboard/:path*",
  ],
};
