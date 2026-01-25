import { NextRequest, NextResponse } from "next/server";
import { getCurrentUser } from "@/lib/session";
import { prisma } from "@/lib/db";
import { JsonValue } from "@prisma/client/runtime/library";

interface NotificationPreferences {
  browser_notifications: boolean;
  notification_sound: boolean;
}

function parsePreferences(json: JsonValue): NotificationPreferences {
  if (typeof json === "object" && json !== null && !Array.isArray(json)) {
    const obj = json as Record<string, unknown>;
    return {
      browser_notifications: typeof obj.browser_notifications === "boolean" ? obj.browser_notifications : true,
      notification_sound: typeof obj.notification_sound === "boolean" ? obj.notification_sound : false,
    };
  }
  return { browser_notifications: true, notification_sound: false };
}

// GET /api/users/me/preferences - Get current user's preferences
export async function GET() {
  const user = await getCurrentUser();
  if (!user) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const dbUser = await prisma.user.findUnique({
    where: { id: user.id },
    select: { notification_preferences: true },
  });

  if (!dbUser) {
    return NextResponse.json({ error: "User not found" }, { status: 404 });
  }

  return NextResponse.json({
    preferences: parsePreferences(dbUser.notification_preferences),
  });
}

// PATCH /api/users/me/preferences - Update current user's preferences
export async function PATCH(request: NextRequest) {
  const user = await getCurrentUser();
  if (!user) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  let body: Partial<NotificationPreferences>;
  try {
    body = await request.json();
  } catch {
    return NextResponse.json({ error: "Invalid request body" }, { status: 400 });
  }

  // Get current preferences
  const dbUser = await prisma.user.findUnique({
    where: { id: user.id },
    select: { notification_preferences: true },
  });

  if (!dbUser) {
    return NextResponse.json({ error: "User not found" }, { status: 404 });
  }

  const currentPrefs = parsePreferences(dbUser.notification_preferences);

  // Merge with updates (only allow specific fields)
  const updatedPrefs = {
    browser_notifications:
      typeof body.browser_notifications === "boolean"
        ? body.browser_notifications
        : currentPrefs.browser_notifications,
    notification_sound:
      typeof body.notification_sound === "boolean"
        ? body.notification_sound
        : currentPrefs.notification_sound,
  };

  // Update the user
  await prisma.user.update({
    where: { id: user.id },
    data: { notification_preferences: updatedPrefs },
  });

  return NextResponse.json({ preferences: updatedPrefs });
}
