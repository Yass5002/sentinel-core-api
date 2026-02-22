ALTER TABLE sequences ADD COLUMN fire_attempts integer DEFAULT 0;
ALTER TABLE sequences ADD COLUMN next_retry_at timestamp with time zone;
