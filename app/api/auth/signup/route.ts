import { NextRequest, NextResponse } from "next/server";
import bcrypt from "bcryptjs";
import { prisma } from "@/lib/db";
import { createSession, setSessionCookie } from "@/lib/session";

export async function POST(request: NextRequest) {
  let body: { org_name?: string; user_name?: string; email?: string; password?: string };
  try {
    body = await request.json();
  } catch {
    return NextResponse.json({ error: "Invalid JSON body" }, { status: 400 });
  }

  const { org_name, user_name, email, password } = body;

  // Validation
  if (!org_name || org_name.trim().length === 0) {
    return NextResponse.json({ error: "org_name is required" }, { status: 400 });
  }

  if (!user_name || user_name.trim().length === 0) {
    return NextResponse.json({ error: "user_name is required" }, { status: 400 });
  }

  if (!email || !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email)) {
    return NextResponse.json({ error: "Valid email is required" }, { status: 400 });
  }

  if (!password || password.length < 8) {
    return NextResponse.json(
      { error: "Password must be at least 8 characters" },
      { status: 400 }
    );
  }

  // Check if email already exists
  const existingUser = await prisma.user.findUnique({ where: { email } });
  if (existingUser) {
    return NextResponse.json({ error: "Email already exists" }, { status: 409 });
  }

  // Hash password with bcrypt (cost factor 12)
  const password_hash = await bcrypt.hash(password, 12);

  // Create Org and User in a transaction
  const { org, user } = await prisma.$transaction(async (tx) => {
    const org = await tx.org.create({
      data: { name: org_name.trim() },
    });

    const user = await tx.user.create({
      data: {
        org_id: org.id,
        email: email.toLowerCase(),
        name: user_name.trim(),
        password_hash,
        role: "admin",
        status: "active",
      },
    });

    return { org, user };
  });

  // Create session and set cookie
  const token = await createSession({
    user_id: user.id,
    org_id: org.id,
    role: user.role,
  });
  await setSessionCookie(token);

  return NextResponse.json(
    {
      user: {
        id: user.id,
        email: user.email,
        name: user.name,
        role: user.role,
      },
      org: {
        id: org.id,
        name: org.name,
      },
    },
    { status: 201 }
  );
}
