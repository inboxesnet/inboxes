import { NextRequest, NextResponse } from "next/server";
import bcrypt from "bcryptjs";
import { prisma } from "@/lib/db";
import { createSession, setSessionCookie } from "@/lib/session";

export async function POST(request: NextRequest) {
  const body = await request.json();
  const { token, password } = body;

  if (!token || typeof token !== "string") {
    return NextResponse.json(
      { error: "Invite token is required" },
      { status: 400 }
    );
  }

  if (!password || typeof password !== "string" || password.length < 8) {
    return NextResponse.json(
      { error: "Password must be at least 8 characters" },
      { status: 400 }
    );
  }

  const user = await prisma.user.findFirst({
    where: { invite_token: token },
  });

  if (!user) {
    return NextResponse.json(
      { error: "Invalid or expired invite token" },
      { status: 400 }
    );
  }

  if (!user.invite_expires_at || user.invite_expires_at < new Date()) {
    return NextResponse.json(
      { error: "Invalid or expired invite token" },
      { status: 400 }
    );
  }

  if (user.status !== "invited") {
    return NextResponse.json(
      { error: "This account has already been claimed" },
      { status: 400 }
    );
  }

  const passwordHash = await bcrypt.hash(password, 12);

  await prisma.user.update({
    where: { id: user.id },
    data: {
      password_hash: passwordHash,
      status: "active",
      claimed_at: new Date(),
      invite_token: null,
      invite_expires_at: null,
    },
  });

  const sessionToken = await createSession({
    user_id: user.id,
    org_id: user.org_id,
    role: user.role,
  });

  await setSessionCookie(sessionToken);

  return NextResponse.json({
    message: "Account claimed successfully",
    user: {
      id: user.id,
      email: user.email,
      name: user.name,
    },
  });
}
