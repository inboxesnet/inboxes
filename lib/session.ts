import { SignJWT, jwtVerify } from "jose";
import { cookies } from "next/headers";
import { prisma } from "@/lib/db";

const SESSION_COOKIE = "session";

function getSecret() {
  const secret = process.env.SESSION_SECRET;
  if (!secret) throw new Error("SESSION_SECRET is not set");
  return new TextEncoder().encode(secret);
}

export interface SessionPayload {
  user_id: string;
  org_id: string;
  role: string;
}

export async function createSession(payload: SessionPayload): Promise<string> {
  const token = await new SignJWT(payload as unknown as Record<string, unknown>)
    .setProtectedHeader({ alg: "HS256" })
    .setIssuedAt()
    .setExpirationTime("7d")
    .sign(getSecret());
  return token;
}

export async function setSessionCookie(token: string) {
  const cookieStore = await cookies();
  cookieStore.set(SESSION_COOKIE, token, {
    httpOnly: true,
    secure: process.env.NODE_ENV === "production",
    sameSite: "lax",
    path: "/",
    maxAge: 60 * 60 * 24 * 7, // 7 days
  });
}

export async function getSession(): Promise<SessionPayload | null> {
  const cookieStore = await cookies();
  const token = cookieStore.get(SESSION_COOKIE)?.value;
  if (!token) return null;

  try {
    const { payload } = await jwtVerify(token, getSecret());
    return payload as unknown as SessionPayload;
  } catch {
    return null;
  }
}

export async function clearSessionCookie() {
  const cookieStore = await cookies();
  cookieStore.delete(SESSION_COOKIE);
}

export interface CurrentUser {
  id: string;
  org_id: string;
  email: string;
  name: string;
  role: string;
  status: string;
}

export async function getCurrentUser(): Promise<CurrentUser | null> {
  const session = await getSession();
  if (!session) return null;

  const user = await prisma.user.findUnique({
    where: { id: session.user_id },
    select: {
      id: true,
      org_id: true,
      email: true,
      name: true,
      role: true,
      status: true,
    },
  });

  if (!user || user.status !== "active") return null;

  return user;
}

/**
 * Checks if the current user is an admin. Returns a 401 JSON response if not authenticated,
 * or a 403 JSON response if authenticated but not an admin.
 * @returns The user if they are an admin, or a NextResponse with an error.
 */
export async function requireAdmin(): Promise<
  CurrentUser | { error: Response }
> {
  const user = await getCurrentUser();

  if (!user) {
    const { NextResponse } = await import("next/server");
    return { error: NextResponse.json({ error: "Unauthorized" }, { status: 401 }) };
  }

  if (user.role !== "admin") {
    const { NextResponse } = await import("next/server");
    return { error: NextResponse.json({ error: "Forbidden" }, { status: 403 }) };
  }

  return user;
}
