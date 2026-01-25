"use client";

import { useState, useEffect, useCallback } from "react";
import { useRouter } from "next/navigation";
import { Input } from "@/components/ui/input";
import { Search, Mail, Loader2 } from "lucide-react";

interface SearchResult {
  id: string;
  thread_id: string;
  subject: string;
  snippet: string;
  from: string;
  date: string;
  folder: string;
}

interface SearchResponse {
  results: SearchResult[];
  count: number;
  query: string;
}

function formatDate(dateStr: string): string {
  const date = new Date(dateStr);
  const now = new Date();
  const diff = now.getTime() - date.getTime();
  const oneDay = 24 * 60 * 60 * 1000;

  if (diff < oneDay && date.getDate() === now.getDate()) {
    return date.toLocaleTimeString([], { hour: "numeric", minute: "2-digit" });
  }

  if (diff < 7 * oneDay) {
    return date.toLocaleDateString([], { weekday: "short" });
  }

  return date.toLocaleDateString([], { month: "short", day: "numeric" });
}

function extractName(address: string): string {
  const match = address.match(/^"?([^"<]+)"?\s*</);
  if (match) return match[1].trim();
  return address.split("@")[0];
}

function highlightMatch(text: string, query: string): React.ReactNode {
  if (!query || !text) return text;

  const terms = query
    .toLowerCase()
    .split(/\s+/)
    .filter((t) => t.length > 0);

  if (terms.length === 0) return text;

  // Create a regex pattern that matches any of the search terms
  const pattern = new RegExp(`(${terms.map(escapeRegex).join("|")})`, "gi");
  const parts = text.split(pattern);

  return parts.map((part, i) => {
    const isMatch = terms.some(
      (term) => part.toLowerCase() === term.toLowerCase()
    );
    return isMatch ? (
      <mark key={i} className="bg-yellow-200 dark:bg-yellow-800 rounded px-0.5">
        {part}
      </mark>
    ) : (
      part
    );
  });
}

function escapeRegex(str: string): string {
  return str.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

export default function SearchPage() {
  const router = useRouter();
  const [query, setQuery] = useState("");
  const [results, setResults] = useState<SearchResult[]>([]);
  const [loading, setLoading] = useState(false);
  const [searched, setSearched] = useState(false);
  const [searchedQuery, setSearchedQuery] = useState("");

  const performSearch = useCallback(async (searchQuery: string) => {
    if (!searchQuery.trim()) {
      setResults([]);
      setSearched(false);
      return;
    }

    setLoading(true);
    setSearched(true);
    setSearchedQuery(searchQuery);

    try {
      const res = await fetch(
        `/api/emails/search?q=${encodeURIComponent(searchQuery)}`
      );
      if (res.ok) {
        const data: SearchResponse = await res.json();
        setResults(data.results);
      } else {
        setResults([]);
      }
    } catch {
      setResults([]);
    } finally {
      setLoading(false);
    }
  }, []);

  // Debounce search after 300ms of no typing
  useEffect(() => {
    if (!query.trim()) {
      setResults([]);
      setSearched(false);
      return;
    }

    const timer = setTimeout(() => {
      performSearch(query);
    }, 300);

    return () => clearTimeout(timer);
  }, [query, performSearch]);

  function handleKeyDown(e: React.KeyboardEvent<HTMLInputElement>) {
    if (e.key === "Enter") {
      performSearch(query);
    }
  }

  function handleResultClick(result: SearchResult) {
    router.push(`/inbox/${result.thread_id}`);
  }

  return (
    <div className="flex flex-col">
      <div className="border-b px-3 py-3 sm:px-4">
        <h1 className="text-lg font-semibold">Search</h1>
      </div>

      <div className="p-3 sm:p-4">
        <div className="relative">
          <Search className="absolute left-3 top-1/2 h-5 w-5 -translate-y-1/2 text-muted-foreground sm:h-4 sm:w-4" />
          <Input
            type="text"
            placeholder="Search emails by keyword..."
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={handleKeyDown}
            className="h-11 pl-10 pr-10 text-base sm:h-10 sm:text-sm"
            autoFocus
          />
          {loading && (
            <Loader2 className="absolute right-3 top-1/2 h-5 w-5 -translate-y-1/2 animate-spin text-muted-foreground sm:h-4 sm:w-4" />
          )}
        </div>
      </div>

      {/* Loading state */}
      {loading && (
        <div className="divide-y">
          {Array.from({ length: 3 }).map((_, i) => (
            <div key={i} className="flex items-start gap-3 px-4 py-3">
              <div className="h-10 w-10 animate-pulse rounded-full bg-muted" />
              <div className="flex-1 space-y-2">
                <div className="h-4 w-1/3 animate-pulse rounded bg-muted" />
                <div className="h-3 w-2/3 animate-pulse rounded bg-muted" />
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Results */}
      {!loading && results.length > 0 && (
        <div className="divide-y">
          {results.map((result) => (
            <div
              key={result.id}
              className="flex min-h-[60px] cursor-pointer items-start gap-3 px-3 py-3 transition-colors hover:bg-muted/50 sm:px-4"
              onClick={() => handleResultClick(result)}
              role="button"
              tabIndex={0}
              onKeyDown={(e) => e.key === "Enter" && handleResultClick(result)}
            >
              <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-full bg-primary/10 text-sm font-medium text-primary">
                <Mail className="h-5 w-5" />
              </div>

              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-2">
                  <span className="truncate text-sm font-medium">
                    {extractName(result.from)}
                  </span>
                  <span className="ml-auto shrink-0 text-xs text-muted-foreground">
                    {formatDate(result.date)}
                  </span>
                </div>

                <p className="truncate text-sm text-foreground">
                  {highlightMatch(result.subject, searchedQuery)}
                </p>

                <p className="mt-0.5 line-clamp-2 text-xs text-muted-foreground">
                  {highlightMatch(result.snippet, searchedQuery)}
                </p>
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Empty state - no results found */}
      {!loading && searched && results.length === 0 && (
        <div className="flex flex-col items-center justify-center py-20 text-center">
          <div className="text-4xl mb-4">🔍</div>
          <h2 className="text-xl font-semibold">No results found</h2>
          <p className="mt-2 text-muted-foreground">
            Try searching with different keywords.
          </p>
        </div>
      )}

      {/* Initial state - no search yet */}
      {!loading && !searched && (
        <div className="flex flex-col items-center justify-center py-20 text-center">
          <div className="text-4xl mb-4">🔎</div>
          <h2 className="text-xl font-semibold">Search your emails</h2>
          <p className="mt-2 text-muted-foreground">
            Enter keywords to find emails by subject or content.
          </p>
        </div>
      )}
    </div>
  );
}
