import { NextRequest, NextResponse } from "next/server";
import { getCurrentUser } from "@/lib/session";
import { prisma } from "@/lib/db";

export async function GET(request: NextRequest) {
  const user = await getCurrentUser();
  if (!user) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { searchParams } = request.nextUrl;
  const query = searchParams.get("q");

  if (!query || query.trim().length === 0) {
    return NextResponse.json(
      { error: "Search query is required" },
      { status: 400 }
    );
  }

  // Optional filters
  const folder = searchParams.get("folder"); // inbox, sent, archive, trash
  const fromAddress = searchParams.get("from");
  const dateFrom = searchParams.get("date_from");
  const dateTo = searchParams.get("date_to");

  // Convert query to tsquery format
  // Split by spaces and join with & for AND matching
  const tsQuery = query
    .trim()
    .split(/\s+/)
    .filter((word) => word.length > 0)
    .map((word) => word.replace(/[^\w]/g, "")) // Remove special chars
    .filter((word) => word.length > 0)
    .join(" & ");

  if (!tsQuery) {
    return NextResponse.json(
      { error: "Invalid search query" },
      { status: 400 }
    );
  }

  // Build WHERE conditions for raw SQL
  const conditions: string[] = ['"user_id" = $1'];
  const params: (string | Date)[] = [user.id];
  let paramIndex = 2;

  // Full-text search condition
  conditions.push(`"search_vector" @@ to_tsquery('english', $${paramIndex})`);
  params.push(tsQuery);
  paramIndex++;

  // Optional folder filter
  if (folder && ["inbox", "sent", "archive", "trash"].includes(folder)) {
    conditions.push(`"folder" = $${paramIndex}`);
    params.push(folder);
    paramIndex++;
  }

  // Optional from address filter (case-insensitive partial match)
  if (fromAddress) {
    conditions.push(`LOWER("from_address") LIKE $${paramIndex}`);
    params.push(`%${fromAddress.toLowerCase()}%`);
    paramIndex++;
  }

  // Optional date range filters
  if (dateFrom) {
    const from = new Date(dateFrom);
    if (!isNaN(from.getTime())) {
      conditions.push(`"received_at" >= $${paramIndex}`);
      params.push(from);
      paramIndex++;
    }
  }

  if (dateTo) {
    const to = new Date(dateTo);
    if (!isNaN(to.getTime())) {
      conditions.push(`"received_at" <= $${paramIndex}`);
      params.push(to);
      paramIndex++;
    }
  }

  const whereClause = conditions.join(" AND ");

  // Execute raw SQL query for full-text search with ranking
  // ts_rank scores results by relevance, combined with recency
  const results = await prisma.$queryRawUnsafe<
    Array<{
      id: string;
      thread_id: string;
      subject: string;
      body_plain: string;
      from_address: string;
      received_at: Date;
      folder: string;
      rank: number;
    }>
  >(
    `SELECT
      "id",
      "thread_id",
      "subject",
      "body_plain",
      "from_address",
      "received_at",
      "folder",
      ts_rank("search_vector", to_tsquery('english', $2)) AS rank
    FROM "Email"
    WHERE ${whereClause}
    ORDER BY rank DESC, "received_at" DESC
    LIMIT 50`,
    ...params
  );

  // Format results with snippets
  const formattedResults = results.map((email) => {
    // Create a snippet around the matching text
    const snippet = createSnippet(email.body_plain, query, 150);

    return {
      id: email.id,
      thread_id: email.thread_id,
      subject: email.subject,
      snippet,
      from: email.from_address,
      date: email.received_at,
      folder: email.folder,
    };
  });

  return NextResponse.json({
    results: formattedResults,
    count: formattedResults.length,
    query,
  });
}

/**
 * Creates a snippet of text around the first occurrence of any search term
 */
function createSnippet(
  text: string,
  query: string,
  maxLength: number
): string {
  if (!text) return "";

  const lowerText = text.toLowerCase();
  const terms = query
    .toLowerCase()
    .split(/\s+/)
    .filter((t) => t.length > 0);

  // Find the first occurrence of any search term
  let firstMatchIndex = -1;
  for (const term of terms) {
    const index = lowerText.indexOf(term);
    if (index !== -1 && (firstMatchIndex === -1 || index < firstMatchIndex)) {
      firstMatchIndex = index;
    }
  }

  if (firstMatchIndex === -1) {
    // No match found, return beginning of text
    return text.length > maxLength
      ? text.substring(0, maxLength).trim() + "..."
      : text;
  }

  // Calculate snippet boundaries
  const start = Math.max(0, firstMatchIndex - Math.floor(maxLength / 3));
  const end = Math.min(text.length, start + maxLength);

  let snippet = text.substring(start, end).trim();

  // Add ellipsis if needed
  if (start > 0) snippet = "..." + snippet;
  if (end < text.length) snippet = snippet + "...";

  return snippet;
}
