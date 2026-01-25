import { NextRequest, NextResponse } from "next/server";
import { prisma } from "@/lib/db";

export async function GET(request: NextRequest) {
  const { searchParams } = new URL(request.url);
  const token = searchParams.get("token");

  if (!token) {
    return NextResponse.json(
      { error: "Token is required" },
      { status: 400 }
    );
  }

  const user = await prisma.user.findFirst({
    where: { invite_token: token },
    select: {
      id: true,
      email: true,
      name: true,
      status: true,
      invite_expires_at: true,
    },
  });

  if (!user) {
    return NextResponse.json(
      { error: "Invalid or expired invite token", valid: false },
      { status: 400 }
    );
  }

  if (!user.invite_expires_at || user.invite_expires_at < new Date()) {
    return NextResponse.json(
      { error: "This invite has expired", valid: false },
      { status: 400 }
    );
  }

  if (user.status !== "invited") {
    return NextResponse.json(
      { error: "This account has already been claimed", valid: false },
      { status: 400 }
    );
  }

  return NextResponse.json({
    valid: true,
    user: {
      name: user.name,
      email: user.email,
    },
  });
}
