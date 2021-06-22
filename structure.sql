create table if not exists ref (
id text PRIMARY KEY,
created_at bigint not null,
name text not null,
dst text not null,
request_addr text not null,
user_agent text
);

ALTER TABLE ref
ADD COLUMN IF NOT EXISTS continent text
ADD COLUMN IF NOT EXISTS country text
ADD COLUMN IF NOT EXISTS region text
ADD COLUMN IF NOT EXISTS city text
ADD COLUMN IF NOT EXISTS zip text
ADD COLUMN IF NOT EXISTS latitude double precision
ADD COLUMN IF NOT EXISTS longitude double precision;