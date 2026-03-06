import { gemoji } from "gemoji";

const emojiMap = new Map<string, string>();
for (const entry of gemoji) {
  for (const name of entry.names) {
    emojiMap.set(name, entry.emoji);
  }
}

export const emojiKeys = [...emojiMap.keys()].sort();

export function lookupEmoji(shortcode: string): string | undefined {
  return emojiMap.get(shortcode);
}

export function filterEmoji(
  query: string,
  limit = 8
): { shortcode: string; emoji: string }[] {
  const q = query.toLowerCase();
  const results: { shortcode: string; emoji: string }[] = [];
  for (const key of emojiKeys) {
    if (key.startsWith(q)) {
      results.push({ shortcode: key, emoji: emojiMap.get(key)! });
      if (results.length >= limit) break;
    }
  }
  return results;
}
