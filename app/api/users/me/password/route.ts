import { NextRequest, NextResponse } from "next/server";
import bcrypt from "bcryptjs";
import { getCurrentUser } from "@/lib/session";
import { prisma } from "@/lib/db";

// PATCH /api/users/me/password - Change current user's password
export async function PATCH(request: NextRequest) {
  const user = await getCurrentUser();
  if (!user) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  let body: { current_password?: string; new_password?: string };
  try {
    body = await request.json();
  } catch {
    return NextResponse.json({ error: "Invalid request body" }, { status: 400 });
  }

  // Validate inputs
  if (!body.current_password || typeof body.current_password !== "string") {
    return NextResponse.json({ error: "Current password is required" }, { status: 400 });
  }
  if (!body.new_password || typeof body.new_password !== "string") {
    return NextResponse.json({ error: "New password is required" }, { status: 400 });
  }
  if (body.new_password.length < 8) {
    return NextResponse.json({ error: "New password must be at least 8 characters" }, { status: 400 });
  }

  // Get user's current password hash
  const dbUser = await prisma.user.findUnique({
    where: { id: user.id },
    select: { password_hash: true },
  });

  if (!dbUser) {
    return NextResponse.json({ error: "User not found" }, { status: 404 });
  }

  // Verify current password
  const isValidPassword = await bcrypt.compare(body.current_password, dbUser.password_hash);
  if (!isValidPassword) {
    return NextResponse.json({ error: "Current password is incorrect" }, { status: 400 });
  }

  // Hash new password with bcrypt (cost factor 12)
  const newPasswordHash = await bcrypt.hash(body.new_password, 12);

  // Update password
  await prisma.user.update({
    where: { id: user.id },
    data: { password_hash: newPasswordHash },
  });

  return NextResponse.json({ success: true, message: "Password updated successfully" });
}
