-- Add tsvector column for full-text search on Email
ALTER TABLE "Email" ADD COLUMN "search_vector" tsvector;

-- Create GIN index for efficient full-text search
CREATE INDEX "Email_search_vector_idx" ON "Email" USING GIN ("search_vector");

-- Create function to update search_vector
CREATE OR REPLACE FUNCTION email_search_vector_update() RETURNS trigger AS $$
BEGIN
  NEW.search_vector :=
    setweight(to_tsvector('english', COALESCE(NEW.subject, '')), 'A') ||
    setweight(to_tsvector('english', COALESCE(NEW.body_plain, '')), 'B') ||
    setweight(to_tsvector('english', COALESCE(NEW.from_address, '')), 'C');
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Create trigger to automatically update search_vector on insert/update
CREATE TRIGGER email_search_vector_trigger
  BEFORE INSERT OR UPDATE ON "Email"
  FOR EACH ROW
  EXECUTE FUNCTION email_search_vector_update();

-- Backfill existing rows (if any)
UPDATE "Email" SET search_vector =
  setweight(to_tsvector('english', COALESCE(subject, '')), 'A') ||
  setweight(to_tsvector('english', COALESCE(body_plain, '')), 'B') ||
  setweight(to_tsvector('english', COALESCE(from_address, '')), 'C');
