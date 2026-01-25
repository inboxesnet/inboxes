import { NextRequest, NextResponse } from "next/server";
import { randomBytes } from "crypto";
import { prisma } from "@/lib/db";
import { getCurrentUser } from "@/lib/session";

export async function POST(
  request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const user = await getCurrentUser();
  if (!user) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  // Admin-only endpoint
  if (user.role !== "admin") {
    return NextResponse.json({ error: "Forbidden" }, { status: 403 });
  }

  const { id } = await params;

  // Find the user to reinvite
  const targetUser = await prisma.user.findFirst({
    where: {
      id,
      org_id: user.org_id,
    },
  });

  if (!targetUser) {
    return NextResponse.json({ error: "User not found" }, { status: 404 });
  }

  // Can only reinvite users with status=invited
  if (targetUser.status !== "invited") {
    return NextResponse.json(
      { error: "Can only reinvite users with invited status" },
      { status: 400 }
    );
  }

  // Generate new invite token (7 days expiry)
  const inviteToken = randomBytes(32).toString("hex");
  const inviteExpiresAt = new Date(Date.now() + 7 * 24 * 60 * 60 * 1000);

  // Update user with new token
  const updatedUser = await prisma.user.update({
    where: { id: targetUser.id },
    data: {
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
          to: targetUser.email,
          subject: `Reminder: You've been invited to join ${org?.name || "the team"} on Inboxes`,
          html: `
            <p>Hi ${targetUser.name},</p>
            <p>This is a reminder that you've been invited to join <strong>${org?.name || "the team"}</strong> on Inboxes.</p>
            <p>Click the link below to set up your account:</p>
            <p><a href="${claimUrl}">${claimUrl}</a></p>
            <p>This invitation expires in 7 days.</p>
            <p>If you weren't expecting this invitation, you can safely ignore this email.</p>
          `,
        }),
      });
    }
  } catch (error) {
    console.error("Failed to send reinvite email:", error);
    // Don't fail the request if email fails - token is updated
  }

  return NextResponse.json({
    id: updatedUser.id,
    email: updatedUser.email,
    name: updatedUser.name,
    role: updatedUser.role,
    status: updatedUser.status,
    invite_expires_at: updatedUser.invite_expires_at,
  });
}
