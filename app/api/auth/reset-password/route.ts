import { NextRequest, NextResponse } from "next/server";
import bcrypt from "bcryptjs";
import { prisma } from "@/lib/db";

export async function POST(request: NextRequest) {
  const body = await request.json();
  const { token, new_password } = body;

  if (!token || typeof token !== "string") {
    return NextResponse.json(
      { error: "Reset token is required" },
      { status: 400 }
    );
  }

  if (!new_password || typeof new_password !== "string" || new_password.length < 8) {
    return NextResponse.json(
      { error: "Password must be at least 8 characters" },
      { status: 400 }
    );
  }

  const user = await prisma.user.findFirst({
    where: { reset_token: token },
  });

  if (!user) {
    return NextResponse.json(
      { error: "Invalid or expired reset token" },
      { status: 400 }
    );
  }

  if (!user.reset_token_expires_at || user.reset_token_expires_at < new Date()) {
    return NextResponse.json(
      { error: "Invalid or expired reset token" },
      { status: 400 }
    );
  }

  const passwordHash = await bcrypt.hash(new_password, 12);

  await prisma.user.update({
    where: { id: user.id },
    data: {
      password_hash: passwordHash,
      reset_token: null,
      reset_token_expires_at: null,
    },
  });

  return NextResponse.json({
    message: "Password has been reset successfully",
  });
}
