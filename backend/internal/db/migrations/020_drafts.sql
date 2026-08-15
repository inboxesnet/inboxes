-- +goose Up

-- Threading context for reply/forward drafts. Without these columns a reply
-- sent from a draft loses its In-Reply-To and References headers and starts
-- a new conversation at the recipient.
ALTER TABLE drafts ADD COLUMN in_reply_to TEXT NOT NULL DEFAULT '';
ALTER TABLE drafts ADD COLUMN references_header TEXT NOT NULL DEFAULT '';
