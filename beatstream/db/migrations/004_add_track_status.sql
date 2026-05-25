-- Phase 2: add upload pipeline status column.
-- Default 'ready' keeps existing rows accessible without re-processing.
ALTER TABLE tracks ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'ready';
