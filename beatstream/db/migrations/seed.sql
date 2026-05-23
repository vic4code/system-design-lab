-- Sample seed data for local development
-- Run: docker compose exec postgres psql -U user -d beatstream -f /migrations/seed.sql

INSERT INTO artists (id, name) VALUES
  ('11111111-1111-1111-1111-111111111111', 'Radiohead'),
  ('22222222-2222-2222-2222-222222222222', 'Portishead'),
  ('33333333-3333-3333-3333-333333333333', 'Massive Attack')
ON CONFLICT DO NOTHING;
