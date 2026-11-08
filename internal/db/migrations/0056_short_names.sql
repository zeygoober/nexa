-- How many one-character usernames an account may still take, one for each
-- gift link of that kind it claimed. The name is chosen when it is used.
ALTER TABLE users ADD COLUMN short_name_credits INTEGER NOT NULL DEFAULT 0;
