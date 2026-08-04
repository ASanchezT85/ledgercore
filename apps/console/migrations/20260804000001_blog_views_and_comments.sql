-- +goose Up
-- Public content schema owned by the console: post view counters and reader
-- comments. No tenant data, no money data — this schema is deliberately
-- isolated from the four service schemas (see infra/postgres/init/01-init.sql).

-- ---------------------------------------------------------------------------
-- Views. One row per (post, visitor, day): a reload does not inflate the count
-- and we never store a raw IP — only a salted hash. "Views" therefore means
-- unique readers per day, which is the number we are willing to publish.
-- ---------------------------------------------------------------------------
CREATE TABLE blog.post_views (
    slug         text        NOT NULL,
    visitor_hash text        NOT NULL,
    day          date        NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (slug, visitor_hash, day)
);

CREATE INDEX post_views_slug_idx ON blog.post_views (slug);

-- ---------------------------------------------------------------------------
-- Comments. Two levels only: a top-level comment, or a reply to one. The check
-- that a parent is itself top-level is enforced in the trigger below, because a
-- plain FK cannot express "the parent must have parent_id IS NULL".
-- ---------------------------------------------------------------------------
CREATE TABLE blog.comments (
    id           uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    slug         text        NOT NULL,
    parent_id    uuid        REFERENCES blog.comments (id) ON DELETE CASCADE,
    author_name  text        NOT NULL CHECK (length(btrim(author_name)) BETWEEN 2 AND 60),
    body         text        NOT NULL CHECK (length(btrim(body)) BETWEEN 2 AND 4000),
    author_hash  text        NOT NULL,
    status       text        NOT NULL DEFAULT 'visible'
                             CHECK (status IN ('visible', 'hidden')),
    created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX comments_slug_created_idx
    ON blog.comments (slug, created_at)
    WHERE status = 'visible';

CREATE INDEX comments_parent_idx ON blog.comments (parent_id);

-- Rate limiting lives in the database, not in process memory: the console runs
-- as a stateless container and an in-memory counter would reset on every deploy.
CREATE INDEX comments_author_hash_created_idx
    ON blog.comments (author_hash, created_at DESC);

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION blog.enforce_single_level_replies()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    parent_slug   text;
    parent_parent uuid;
BEGIN
    IF NEW.parent_id IS NULL THEN
        RETURN NEW;
    END IF;

    SELECT slug, parent_id INTO parent_slug, parent_parent
    FROM blog.comments WHERE id = NEW.parent_id;

    IF parent_slug IS NULL THEN
        RAISE EXCEPTION 'parent comment % does not exist', NEW.parent_id;
    END IF;
    IF parent_parent IS NOT NULL THEN
        RAISE EXCEPTION 'replies are one level deep: % is already a reply', NEW.parent_id;
    END IF;
    IF parent_slug <> NEW.slug THEN
        RAISE EXCEPTION 'reply slug % does not match parent slug %', NEW.slug, parent_slug;
    END IF;

    RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER comments_single_level_replies
    BEFORE INSERT OR UPDATE ON blog.comments
    FOR EACH ROW EXECUTE FUNCTION blog.enforce_single_level_replies();

-- +goose Down
DROP TRIGGER IF EXISTS comments_single_level_replies ON blog.comments;
DROP FUNCTION IF EXISTS blog.enforce_single_level_replies();
DROP TABLE IF EXISTS blog.comments;
DROP TABLE IF EXISTS blog.post_views;
