export function extractSender(emails: string[]): string {
  if (!emails || emails.length === 0) return "Unknown";
  const first = emails[0];
  const atIndex = first.indexOf("@");
  return atIndex > 0 ? first.substring(0, atIndex) : first;
}

export function decodeHtmlEntities(text: string): string {
  return text
    .replace(/&#x([0-9a-fA-F]+);/g, (_, hex) =>
      String.fromCharCode(parseInt(hex, 16))
    )
    .replace(/&#(\d+);/g, (_, dec) =>
      String.fromCharCode(parseInt(dec, 10))
    )
    .replace(/&amp;/g, "&")
    .replace(/&lt;/g, "<")
    .replace(/&gt;/g, ">")
    .replace(/&quot;/g, '"')
    .replace(/&apos;/g, "'");
}

export function cleanSnippet(text: string): string {
  return decodeHtmlEntities(text)
    .replace(/\s+/g, " ")
    .trim();
}

export function parseParticipants(raw: string[] | string): string[] {
  if (Array.isArray(raw)) return raw;
  if (typeof raw === "string") {
    try {
      return JSON.parse(raw);
    } catch {
      return [];
    }
  }
  return [];
}
