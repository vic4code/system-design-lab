# YYYY-MM-DD — [Phase X] [Topic]

## What I ran

One paragraph. Specific commands, exact config values, not vague summaries.

```bash
# example
docker compose up -d
k6 run -e TRACK_ID=abc123 k6/load.js
```

## What broke

The exact error message. Your first (wrong) hypothesis about the cause.

## How I found the root cause

Commands used. Logs read. Metrics checked. What ruled out your first hypothesis.

```bash
# example
docker compose logs api-1 | grep ERROR
```

## Root cause

The real explanation, tied to a concept from the README's Core Concepts section.

## What I'd say in an interview

> "I actually built this and ran into X. The root cause was Y,
> which is why in production you need to..."

## Open questions

Things this raised that you don't understand yet. These become your next reading targets.

- [ ] ?
- [ ] ?
