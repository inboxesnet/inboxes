"use client";

import * as React from "react";
import { X, Loader2 } from "lucide-react";
import { Input } from "@/components/ui/input";

interface Contact {
  email: string;
  name: string | null;
  frequency: number;
}

interface RecipientAutocompleteProps {
  id?: string;
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  className?: string;
}

export function RecipientAutocomplete({
  id,
  value,
  onChange,
  placeholder,
  className,
}: RecipientAutocompleteProps) {
  const [inputValue, setInputValue] = React.useState("");
  const [suggestions, setSuggestions] = React.useState<Contact[]>([]);
  const [showSuggestions, setShowSuggestions] = React.useState(false);
  const [loading, setLoading] = React.useState(false);
  const [selectedIndex, setSelectedIndex] = React.useState(-1);
  const inputRef = React.useRef<HTMLInputElement>(null);
  const containerRef = React.useRef<HTMLDivElement>(null);
  const debounceRef = React.useRef<NodeJS.Timeout | null>(null);

  // Parse value into array of emails
  const emails = React.useMemo(() => {
    return value
      .split(",")
      .map((e) => e.trim())
      .filter(Boolean);
  }, [value]);

  // Handle clicks outside to close dropdown
  React.useEffect(() => {
    function handleClickOutside(event: MouseEvent) {
      if (containerRef.current && !containerRef.current.contains(event.target as Node)) {
        setShowSuggestions(false);
      }
    }
    document.addEventListener("mousedown", handleClickOutside);
    return () => document.removeEventListener("mousedown", handleClickOutside);
  }, []);

  // Fetch suggestions with debounce
  const fetchSuggestions = React.useCallback(async (query: string) => {
    if (query.length < 2) {
      setSuggestions([]);
      return;
    }

    setLoading(true);
    try {
      const res = await fetch(`/api/contacts/suggest?q=${encodeURIComponent(query)}`);
      if (res.ok) {
        const data = await res.json();
        // Filter out already selected emails
        const currentEmails = new Set(emails.map((e) => e.toLowerCase()));
        const filtered = (data.contacts as Contact[]).filter(
          (c) => !currentEmails.has(c.email.toLowerCase())
        );
        setSuggestions(filtered);
      }
    } catch {
      setSuggestions([]);
    } finally {
      setLoading(false);
    }
  }, [emails]);

  // Handle input change with debounce
  function handleInputChange(e: React.ChangeEvent<HTMLInputElement>) {
    const newValue = e.target.value;
    setInputValue(newValue);
    setSelectedIndex(-1);

    // Check if user typed a comma or hit space after valid email
    const lastChar = newValue.slice(-1);
    if (lastChar === "," || lastChar === " ") {
      const trimmedValue = newValue.slice(0, -1).trim();
      if (trimmedValue && isValidEmail(trimmedValue)) {
        addEmail(trimmedValue);
        return;
      }
    }

    // Debounce search
    if (debounceRef.current) {
      clearTimeout(debounceRef.current);
    }

    debounceRef.current = setTimeout(() => {
      const query = newValue.trim();
      if (query.length >= 2) {
        setShowSuggestions(true);
        fetchSuggestions(query);
      } else {
        setSuggestions([]);
        setShowSuggestions(false);
      }
    }, 300);
  }

  function isValidEmail(email: string): boolean {
    return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email);
  }

  function addEmail(email: string) {
    const trimmed = email.trim().toLowerCase();
    if (!trimmed) return;

    // Check if already added
    if (emails.some((e) => e.toLowerCase() === trimmed)) {
      setInputValue("");
      return;
    }

    const newEmails = [...emails, trimmed];
    onChange(newEmails.join(", "));
    setInputValue("");
    setSuggestions([]);
    setShowSuggestions(false);
  }

  function removeEmail(index: number) {
    const newEmails = emails.filter((_, i) => i !== index);
    onChange(newEmails.join(", "));
  }

  function selectSuggestion(contact: Contact) {
    addEmail(contact.email);
    inputRef.current?.focus();
  }

  function handleKeyDown(e: React.KeyboardEvent<HTMLInputElement>) {
    if (e.key === "Enter") {
      e.preventDefault();
      if (selectedIndex >= 0 && selectedIndex < suggestions.length) {
        selectSuggestion(suggestions[selectedIndex]);
      } else if (inputValue.trim() && isValidEmail(inputValue.trim())) {
        addEmail(inputValue.trim());
      }
    } else if (e.key === "ArrowDown") {
      e.preventDefault();
      setSelectedIndex((prev) =>
        prev < suggestions.length - 1 ? prev + 1 : prev
      );
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      setSelectedIndex((prev) => (prev > 0 ? prev - 1 : -1));
    } else if (e.key === "Escape") {
      setShowSuggestions(false);
    } else if (e.key === "Backspace" && !inputValue && emails.length > 0) {
      // Remove last email on backspace when input is empty
      removeEmail(emails.length - 1);
    } else if (e.key === "Tab" && inputValue.trim()) {
      // Add email on tab if valid
      if (isValidEmail(inputValue.trim())) {
        e.preventDefault();
        addEmail(inputValue.trim());
      }
    }
  }

  return (
    <div ref={containerRef} className={`relative ${className || ""}`}>
      <div
        className="flex flex-wrap items-center gap-1 min-h-[40px] w-full rounded-md border border-input bg-background px-3 py-1 text-sm ring-offset-background focus-within:ring-2 focus-within:ring-ring focus-within:ring-offset-2 cursor-text"
        onClick={() => inputRef.current?.focus()}
      >
        {/* Email chips */}
        {emails.map((email, index) => (
          <span
            key={index}
            className="inline-flex items-center gap-1 rounded-full bg-primary/10 px-2 py-0.5 text-xs"
          >
            <span className="max-w-[150px] truncate">{email}</span>
            <button
              type="button"
              onClick={(e) => {
                e.stopPropagation();
                removeEmail(index);
              }}
              className="rounded-full p-0.5 hover:bg-primary/20"
            >
              <X className="h-3 w-3" />
            </button>
          </span>
        ))}

        {/* Input field */}
        <div className="flex-1 min-w-[120px]">
          <input
            ref={inputRef}
            id={id}
            type="text"
            value={inputValue}
            onChange={handleInputChange}
            onKeyDown={handleKeyDown}
            onFocus={() => {
              if (suggestions.length > 0) {
                setShowSuggestions(true);
              }
            }}
            placeholder={emails.length === 0 ? placeholder : ""}
            className="w-full bg-transparent py-1 outline-none placeholder:text-muted-foreground"
          />
        </div>

        {/* Loading indicator */}
        {loading && (
          <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
        )}
      </div>

      {/* Suggestions dropdown */}
      {showSuggestions && suggestions.length > 0 && (
        <div className="absolute z-50 mt-1 w-full rounded-md border bg-popover shadow-lg max-h-[200px] overflow-y-auto">
          <div className="p-1">
            {suggestions.map((contact, index) => (
              <button
                key={contact.email}
                type="button"
                onClick={() => selectSuggestion(contact)}
                onMouseEnter={() => setSelectedIndex(index)}
                className={`flex w-full items-center gap-2 rounded-sm px-2 py-1.5 text-sm ${
                  index === selectedIndex ? "bg-accent" : "hover:bg-accent"
                }`}
              >
                <div className="flex-1 text-left">
                  {contact.name ? (
                    <>
                      <span className="font-medium">{contact.name}</span>
                      <span className="ml-1 text-muted-foreground">
                        &lt;{contact.email}&gt;
                      </span>
                    </>
                  ) : (
                    <span>{contact.email}</span>
                  )}
                </div>
                <span className="text-xs text-muted-foreground">
                  {contact.frequency}×
                </span>
              </button>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}
