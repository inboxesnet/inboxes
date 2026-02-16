"use client";

import { useState } from "react";
import { useParams } from "next/navigation";
import { api } from "@/lib/api";
import { ThreadList } from "@/components/thread-list";
import { Input } from "@/components/ui/input";
import { Spinner } from "@/components/ui/spinner";
import { Search } from "lucide-react";
import type { Thread } from "@/lib/types";

const EMPTY_SET = new Set<string>();
const noop = () => {};

export default function SearchPage() {
  const params = useParams();
  const domainId = params.domainId as string;
  const [query, setQuery] = useState("");
  const [results, setResults] = useState<Thread[]>([]);
  const [loading, setLoading] = useState(false);
  const [searched, setSearched] = useState(false);

  async function handleSearch(e: React.FormEvent) {
    e.preventDefault();
    if (!query.trim()) return;

    setLoading(true);
    setSearched(true);
    try {
      const data = await api.get<{ threads: Thread[] }>(
        `/api/emails/search?q=${encodeURIComponent(query)}&domain_id=${domainId}`
      );
      setResults(data.threads || []);
    } catch {
      setResults([]);
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="h-full flex flex-col">
      <div className="border-b px-4 py-3 shrink-0">
        <form onSubmit={handleSearch} className="relative">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
          <Input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Search emails..."
            className="pl-9"
          />
        </form>
      </div>
      <div className="flex-1 overflow-y-auto">
        {loading ? (
          <div className="flex items-center justify-center h-32">
            <Spinner className="h-6 w-6" />
          </div>
        ) : searched && results.length === 0 ? (
          <div className="flex items-center justify-center h-32 text-muted-foreground text-sm">
            No results found
          </div>
        ) : (
          <ThreadList
            threads={results}
            domainId={domainId}
            folder="inbox"
            selectedIds={EMPTY_SET}
            focusedIndex={-1}
            onToggleSelect={noop}
            onToggleSelectAll={noop}
            onStar={noop}
            onAction={noop}
          />
        )}
      </div>
    </div>
  );
}
