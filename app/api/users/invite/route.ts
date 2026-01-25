import { NextRequest, NextResponse } from "next/server";
import { randomBytes } from "crypto";
import { prisma } from "@/lib/db";
import { requireAdmin } from "@/lib/session";

export async function POST(request: NextRequest) {
  const result = await requireAdmin();
  if ("error" in result) return result.error;
  const user = result;

  const body = await request.json();
  const { email, name, role } = body;

  // Validate required fields
  if (!email || typeof email !== "string") {
    return NextResponse.json({ error: "Email is required" }, { status: 400 });
  }

  if (!name || typeof name !== "string") {
    return NextResponse.json({ error: "Name is required" }, { status: 400 });
  }

  // Validate role
  const validRoles = ["admin", "member"];
  const userRole = role && validRoles.includes(role) ? role : "member";

  const normalizedEmail = email.toLowerCase().trim();

  // Validate email format
  const emailRegex = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
  if (!emailRegex.test(normalizedEmail)) {
    return NextResponse.json({ error: "Invalid email format" }, { status: 400 });
  }

  // Get org domain to validate email matches
  const domain = await prisma.domain.findFirst({
    where: { org_id: user.org_id, status: "active" },
  });

  if (domain) {
    const emailDomain = normalizedEmail.split("@")[1];
    if (emailDomain !== domain.domain) {
      return NextResponse.json(
        { error: `Email must be from the organization domain: @${domain.domain}` },
        { status: 400 }
      );
    }
  }

  // Check if email already exists in org
  const existingUser = await prisma.user.findUnique({
    where: { email: normalizedEmail },
  });

  if (existingUser) {
    return NextResponse.json(
      { error: "A user with this email already exists" },
      { status: 409 }
    );
  }

  // Generate invite token (7 days expiry)
  const inviteToken = randomBytes(32).toString("hex");
  const inviteExpiresAt = new Date(Date.now() + 7 * 24 * 60 * 60 * 1000);

  // Create user with invited status and placeholder password
  const newUser = await prisma.user.create({
    data: {
      org_id: user.org_id,
      email: normalizedEmail,
      name: name.trim(),
      password_hash: "", // Will be set when user claims the invite
      role: userRole as "admin" | "member",
      status: "invited",
      invite_token: inviteToken,
      invite_expires_at: inviteExpiresAt,
    },
  });

  // Send invite email via Resend
  const claimUrl = `${process.env.NEXT_PUBLIC_APP_URL || "http://localhost:3000"}/claim?token=${inviteToken}`;

  try {
    const resendApiKey = process.env.RESEND_API_KEY;
    if (resendApiKey) {
      // Get org name for the email
      const org = await prisma.org.findUnique({
        where: { id: user.org_id },
        select: { name: true },
      });

      await fetch("https://api.resend.com/emails", {
        method: "POST",
        headers: {
          Authorization: `Bearer ${resendApiKey}`,
          "Content-Type": "application/json",
        },
        body: JSON.stringify({
          from: "noreply@inboxes.net",
          to: normalizedEmail,
          subject: `You've been invited to join ${org?.name || "the team"} on Inboxes`,
          html: `
            <p>Hi ${name.trim()},</p>
            <p>You've been invited to join <strong>${org?.name || "the team"}</strong> on Inboxes.</p>
            <p>Click the link below to set up your account:</p>
            <p><a href="${claimUrl}">${claimUrl}</a></p>
            <p>This invitation expires in 7 days.</p>
            <p>If you weren't expecting this invitation, you can safely ignore this email.</p>
          `,
        }),
      });
    }
  } catch (error) {
    console.error("Failed to send invite email:", error);
    // Don't fail the request if email fails - user is created
  }

  return NextResponse.json({
    id: newUser.id,
    email: newUser.email,
    name: newUser.name,
    role: newUser.role,
    status: newUser.status,
    invite_expires_at: newUser.invite_expires_at,
  });
}
