"use client";

import { useState, useRef, useCallback, useEffect } from "react";
import { X } from "lucide-react";
import { api } from "@/lib/api";
import { cn } from "@/lib/utils";

interface RecipientInputProps {
  value: string[];
  onChange: (value: string[]) => void;
  placeholder?: string;
  className?: string;
}

interface Suggestion {
  email: string;
  name?: string;
  count: number;
}

export function RecipientInput({
  value,
  onChange,
  placeholder = "recipient@example.com",
  className,
}: RecipientInputProps) {
  const [inputValue, setInputValue] = useState("");
  const [suggestions, setSuggestions] = useState<Suggestion[]>([]);
  const [showSuggestions, setShowSuggestions] = useState(false);
  const [selectedIndex, setSelectedIndex] = useState(-1);
  const inputRef = useRef<HTMLInputElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const fetchTimerRef = useRef<NodeJS.Timeout | null>(null);

  const fetchSuggestions = useCallback(async (query: string) => {
    if (query.length < 2) {
      setSuggestions([]);
      return;
    }
    try {
      const data = await api.get<Suggestion[]>(
        `/api/contacts/suggest?q=${encodeURIComponent(query)}`
      );
      // Filter out already-added recipients
      const filtered = (data || []).filter(
        (s) => !value.includes(s.email)
      );
      setSuggestions(filtered);
      setShowSuggestions(filtered.length > 0);
      setSelectedIndex(-1);
    } catch {
      setSuggestions([]);
    }
  }, [value]);

  const addRecipient = useCallback(
    (email: string) => {
      const trimmed = email.trim().toLowerCase();
      if (trimmed && !value.includes(trimmed)) {
        onChange([...value, trimmed]);
      }
      setInputValue("");
      setSuggestions([]);
      setShowSuggestions(false);
      inputRef.current?.focus();
    },
    [value, onChange]
  );

  const removeRecipient = useCallback(
    (email: string) => {
      onChange(value.filter((v) => v !== email));
    },
    [value, onChange]
  );

  function handleInputChange(e: React.ChangeEvent<HTMLInputElement>) {
    const val = e.target.value;

    // Handle comma/semicolon/space as separator for completed email addresses
    if (val.endsWith(",") || val.endsWith(";")) {
      const email = val.slice(0, -1).trim();
      if (email && email.includes("@")) {
        addRecipient(email);
        return;
      }
    }

    setInputValue(val);

    // Debounced suggestion fetch
    if (fetchTimerRef.current) clearTimeout(fetchTimerRef.current);
    fetchTimerRef.current = setTimeout(() => fetchSuggestions(val.trim()), 200);
  }

  function handleKeyDown(e: React.KeyboardEvent) {
    if (e.key === "Enter" || e.key === "Tab") {
      if (showSuggestions && selectedIndex >= 0 && selectedIndex < suggestions.length) {
        e.preventDefault();
        addRecipient(suggestions[selectedIndex].email);
      } else if (inputValue.trim() && inputValue.includes("@")) {
        e.preventDefault();
        addRecipient(inputValue);
      } else if (e.key === "Tab") {
        // Allow natural tab behavior
        return;
      }
    } else if (e.key === "Backspace" && !inputValue && value.length > 0) {
      removeRecipient(value[value.length - 1]);
    } else if (e.key === "ArrowDown" && showSuggestions) {
      e.preventDefault();
      setSelectedIndex((prev) => Math.min(prev + 1, suggestions.length - 1));
    } else if (e.key === "ArrowUp" && showSuggestions) {
      e.preventDefault();
      setSelectedIndex((prev) => Math.max(prev - 1, 0));
    } else if (e.key === "Escape") {
      setShowSuggestions(false);
    }
  }

  function handleBlur() {
    // Delay to allow click on suggestion
    setTimeout(() => {
      if (inputValue.trim() && inputValue.includes("@")) {
        addRecipient(inputValue);
      }
      setShowSuggestions(false);
    }, 150);
  }

  // Close suggestions on outside click
  useEffect(() => {
    function handleClick(e: MouseEvent) {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        setShowSuggestions(false);
      }
    }
    document.addEventListener("mousedown", handleClick);
    return () => document.removeEventListener("mousedown", handleClick);
  }, []);

  return (
    <div ref={containerRef} className="relative flex-1">
      <div
        className={cn(
          "flex flex-wrap items-center gap-1 min-h-[28px] px-1",
          className
        )}
        onClick={() => inputRef.current?.focus()}
      >
        {value.map((email) => (
          <span
            key={email}
            className="inline-flex items-center gap-0.5 text-xs bg-muted rounded px-1.5 py-0.5 max-w-[200px]"
          >
            <span className="truncate">{email}</span>
            <button
              type="button"
              onClick={(e) => {
                e.stopPropagation();
                removeRecipient(email);
              }}
              className="shrink-0 p-0 hover:text-destructive"
            >
              <X className="h-3 w-3" />
            </button>
          </span>
        ))}
        <input
          ref={inputRef}
          type="text"
          value={inputValue}
          onChange={handleInputChange}
          onKeyDown={handleKeyDown}
          onBlur={handleBlur}
          onFocus={() => {
            if (suggestions.length > 0) setShowSuggestions(true);
          }}
          placeholder={value.length === 0 ? placeholder : ""}
          className="flex-1 min-w-[120px] h-7 text-sm bg-transparent outline-none border-0"
        />
      </div>

      {/* Suggestions dropdown */}
      {showSuggestions && suggestions.length > 0 && (
        <div className="absolute left-0 right-0 top-full z-50 mt-1 rounded-md border bg-popover shadow-md max-h-[200px] overflow-y-auto">
          {suggestions.map((s, i) => (
            <button
              key={s.email}
              type="button"
              onMouseDown={(e) => {
                e.preventDefault();
                addRecipient(s.email);
              }}
              className={cn(
                "w-full text-left px-3 py-1.5 text-sm hover:bg-accent",
                i === selectedIndex && "bg-accent"
              )}
            >
              <span className="font-medium">{s.email}</span>
              {s.name && (
                <span className="text-muted-foreground ml-2">{s.name}</span>
              )}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
