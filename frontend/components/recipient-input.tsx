"use client";

import { useState, useRef, useCallback, useEffect, useId } from "react";
import { toast } from "sonner";
import { X } from "lucide-react";
import { api } from "@/lib/api";
import { cn } from "@/lib/utils";

const EMAIL_REGEX = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

// parseAddresses splits typed or pasted text into bare addresses. It accepts
// "a@x.com, b@y.com", "a@x.com b@y.com" and "Name <a@x.com>".
export function parseAddresses(raw: string): string[] {
  const out: string[] = [];
  for (const part of raw.split(/[,;\n]+/)) {
    const angle = part.match(/<([^<>]+)>/);
    const pieces = angle ? [angle[1]] : part.split(/\s+/);
    for (const piece of pieces) {
      const cleaned = piece.trim().toLowerCase();
      if (cleaned) out.push(cleaned);
    }
  }
  return out;
}

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
  // Counts suggestion requests. A slow, old response must not replace a
  // newer one or open the list again after an add.
  const fetchSeqRef = useRef(0);
  const listId = useId();

  const fetchSuggestions = useCallback(async (query: string) => {
    const seq = ++fetchSeqRef.current;
    if (query.length < 2) {
      setSuggestions([]);
      return;
    }
    try {
      const data = await api.get<Suggestion[]>(
        `/api/contacts/suggest?q=${encodeURIComponent(query)}`
      );
      if (seq !== fetchSeqRef.current) return;
      // Filter out already-added recipients
      const filtered = (data || []).filter(
        (s) => !value.includes(s.email)
      );
      setSuggestions(filtered);
      setShowSuggestions(filtered.length > 0);
      setSelectedIndex(-1);
    } catch {
      if (seq === fetchSeqRef.current) setSuggestions([]);
    }
  }, [value]);

  // addRecipient adds each valid address in the text. Invalid text stays in
  // the input so the user can correct it. `refocus` is false on blur: the
  // focus must stay where the user moved it.
  const addRecipient = useCallback(
    (text: string, refocus = true) => {
      const parsed = parseAddresses(text);
      if (parsed.length === 0) return;
      const valid = parsed.filter((a) => EMAIL_REGEX.test(a));
      const invalid = parsed.filter((a) => !EMAIL_REGEX.test(a));
      if (invalid.length > 0) {
        toast.error(`Invalid email: ${invalid.join(", ")}`);
      }
      const next = [...value];
      for (const address of valid) {
        if (!next.includes(address)) next.push(address);
      }
      if (next.length !== value.length) onChange(next);
      // Cancel a pending suggestion fetch for the text that is now gone.
      if (fetchTimerRef.current) clearTimeout(fetchTimerRef.current);
      fetchSeqRef.current++;
      setInputValue(invalid.join(", "));
      setSuggestions([]);
      setShowSuggestions(false);
      if (refocus) inputRef.current?.focus();
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
      } else if (!e.metaKey && !e.ctrlKey) {
        // A plain Enter in this field must not submit the form and send
        // the email. Cmd/Ctrl+Enter still reaches the form.
        e.preventDefault();
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
      // preventDefault marks the key as consumed so the compose window
      // does not also close on the same press.
      if (showSuggestions) e.preventDefault();
      setShowSuggestions(false);
    }
  }

  function handleBlur() {
    // Commit at once. With a delay, a click on Send ran before the typed
    // address was added. Suggestions use onMouseDown with preventDefault,
    // so a click on one does not blur the input.
    if (inputValue.trim() && inputValue.includes("@")) {
      addRecipient(inputValue, false);
    }
    setShowSuggestions(false);
  }

  // Clean up timers on unmount
  useEffect(() => {
    return () => {
      if (fetchTimerRef.current) clearTimeout(fetchTimerRef.current);
    };
  }, []);

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
              aria-label={`Remove ${email}`}
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
          role="combobox"
          aria-autocomplete="list"
          aria-expanded={showSuggestions && suggestions.length > 0}
          aria-controls={listId}
          aria-activedescendant={selectedIndex >= 0 ? `${listId}-option-${selectedIndex}` : undefined}
        />
      </div>

      {/* Suggestions dropdown */}
      {showSuggestions && suggestions.length > 0 && (
        <div id={listId} role="listbox" className="absolute left-0 right-0 top-full z-50 mt-1 rounded-md border bg-popover shadow-md max-h-[200px] overflow-y-auto">
          {suggestions.map((s, i) => (
            <button
              key={s.email}
              id={`${listId}-option-${i}`}
              type="button"
              role="option"
              aria-selected={i === selectedIndex}
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
