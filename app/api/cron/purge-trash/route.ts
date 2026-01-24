import { NextRequest, NextResponse } from "next/server";
import { prisma } from "@/lib/db";

/**
 * POST /api/cron/purge-trash
 *
 * Scheduled job to permanently delete emails and threads that have been in trash
 * for more than 30 days (trash_expires_at < now).
 *
 * Protected by CRON_SECRET header to prevent unauthorized calls.
 * Designed to be called by Vercel Cron (once daily).
 */
export async function POST(request: NextRequest) {
  // Verify the cron secret to prevent unauthorized calls
  const authHeader = request.headers.get("authorization");
  const cronSecret = process.env.CRON_SECRET;

  if (!cronSecret) {
    console.error("CRON_SECRET environment variable not set");
    return NextResponse.json(
      { error: "Server configuration error" },
      { status: 500 }
    );
  }

  if (authHeader !== `Bearer ${cronSecret}`) {
    return NextResponse.json(
      { error: "Unauthorized" },
      { status: 401 }
    );
  }

  const now = new Date();

  try {
    // Find all expired emails in trash
    const expiredEmails = await prisma.email.findMany({
      where: {
        folder: "trash",
        trash_expires_at: {
          lt: now,
        },
      },
      select: {
        id: true,
        thread_id: true,
      },
    });

    if (expiredEmails.length === 0) {
      console.log("Purge trash: No expired emails found");
      return NextResponse.json({
        success: true,
        purged_emails: 0,
        purged_threads: 0,
      });
    }

    // Get unique thread IDs
    const threadIds = Array.from(new Set(expiredEmails.map((e) => e.thread_id)));
    const emailIds = expiredEmails.map((e) => e.id);

    // Delete the expired emails
    const deletedEmails = await prisma.email.deleteMany({
      where: {
        id: {
          in: emailIds,
        },
      },
    });

    // For each thread, check if it's now empty and delete if so
    let purgedThreadCount = 0;
    for (const threadId of threadIds) {
      const remainingEmails = await prisma.email.count({
        where: {
          thread_id: threadId,
        },
      });

      if (remainingEmails === 0) {
        await prisma.thread.delete({
          where: {
            id: threadId,
          },
        });
        purgedThreadCount++;
      } else {
        // Update thread message_count to reflect remaining emails
        await prisma.thread.update({
          where: {
            id: threadId,
          },
          data: {
            message_count: remainingEmails,
          },
        });
      }
    }

    console.log(
      `Purge trash: Deleted ${deletedEmails.count} emails and ${purgedThreadCount} threads`
    );

    return NextResponse.json({
      success: true,
      purged_emails: deletedEmails.count,
      purged_threads: purgedThreadCount,
    });
  } catch (error) {
    console.error("Purge trash error:", error);
    return NextResponse.json(
      { error: "Internal server error" },
      { status: 500 }
    );
  }
}
