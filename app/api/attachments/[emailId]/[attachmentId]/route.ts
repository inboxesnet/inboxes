import { NextRequest, NextResponse } from "next/server";
import { getCurrentUser } from "@/lib/session";
import { prisma } from "@/lib/db";

interface StoredAttachment {
  id: string;
  filename: string;
  content_type: string;
  size: number;
  content?: string | null;
  url?: string;
}

export async function GET(
  request: NextRequest,
  { params }: { params: Promise<{ emailId: string; attachmentId: string }> }
) {
  const user = await getCurrentUser();
  if (!user) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { emailId, attachmentId } = await params;

  // Find the email belonging to this user
  const email = await prisma.email.findFirst({
    where: {
      id: emailId,
      user_id: user.id,
    },
    select: {
      attachments: true,
    },
  });

  if (!email) {
    return NextResponse.json({ error: "Email not found" }, { status: 404 });
  }

  // Parse attachments JSON
  let attachments: StoredAttachment[] = [];
  try {
    const raw = email.attachments;
    if (typeof raw === "string") {
      attachments = JSON.parse(raw) as StoredAttachment[];
    } else if (Array.isArray(raw)) {
      attachments = raw as unknown as StoredAttachment[];
    }
  } catch {
    return NextResponse.json({ error: "Invalid attachment data" }, { status: 500 });
  }

  // Find the specific attachment
  const attachment = attachments.find((a) => a.id === attachmentId);
  if (!attachment) {
    return NextResponse.json({ error: "Attachment not found" }, { status: 404 });
  }

  // Check if we have content stored (base64)
  if (attachment.content) {
    // Decode base64 and return as file
    const buffer = Buffer.from(attachment.content, "base64");

    return new NextResponse(buffer, {
      status: 200,
      headers: {
        "Content-Type": attachment.content_type || "application/octet-stream",
        "Content-Disposition": `attachment; filename="${encodeURIComponent(attachment.filename)}"`,
        "Content-Length": buffer.length.toString(),
      },
    });
  }

  // Check if we have a URL stored (for outbound attachments uploaded to S3)
  if (attachment.url) {
    // Redirect to the URL
    return NextResponse.redirect(attachment.url);
  }

  return NextResponse.json(
    { error: "Attachment content not available" },
    { status: 404 }
  );
}
