#!/usr/bin/env bash
# Seed demo tracks into beatstream via the API.
# Prerequisites: docker compose up, make migrate
# Usage: cd demo && ./seed-demo-tracks.sh

set -euo pipefail

API="https://localhost/v1"
CURL="curl -sk"

echo "==> Registering demo user..."
TOKEN=$($CURL "$API/auth/register" \
  -H "Content-Type: application/json" \
  -d '{"email":"demo@beatstream.io","password":"demo1234","name":"Demo User"}' \
  | python3 -c "import json,sys; print(json.load(sys.stdin).get('token',''))" 2>/dev/null || true)

if [ -z "$TOKEN" ]; then
  echo "    User might already exist, logging in..."
  TOKEN=$($CURL "$API/auth/login" \
    -H "Content-Type: application/json" \
    -d '{"email":"demo@beatstream.io","password":"demo1234"}' \
    | python3 -c "import json,sys; print(json.load(sys.stdin)['token'])")
fi

echo "    Token: ${TOKEN:0:20}..."

echo ""
echo "==> Creating artists and uploading tracks..."

# Read tracks.json and upload each track
python3 -c "
import json, subprocess, sys

with open('audio/tracks.json') as f:
    tracks = json.loads(f.read())

api = '${API}'
token = '${TOKEN}'
artists_cache = {}

for i, track in enumerate(tracks, 1):
    artist_name = track['artist']
    title = track['title']
    filename = 'audio/' + track['filename']

    # Create artist if not cached
    if artist_name not in artists_cache:
        result = subprocess.run([
            'curl', '-sk', f'{api}/artists',
            '-H', f'Authorization: Bearer {token}',
            '-H', 'Content-Type: application/json',
            '-d', json.dumps({'name': artist_name})
        ], capture_output=True, text=True)

        try:
            data = json.loads(result.stdout)
            artists_cache[artist_name] = data['id']
        except (json.JSONDecodeError, KeyError):
            # Artist might already exist, try to find it
            result2 = subprocess.run([
                'curl', '-sk', f'{api}/artists'
            ], capture_output=True, text=True)
            try:
                all_artists = json.loads(result2.stdout)
                for a in all_artists.get('items', []):
                    if a['name'] == artist_name:
                        artists_cache[artist_name] = a['id']
                        break
            except:
                print(f'  [{i}/20] SKIP {title} - cannot find/create artist')
                continue

            if artist_name not in artists_cache:
                print(f'  [{i}/20] SKIP {title} - artist creation failed')
                continue

    artist_id = artists_cache[artist_name]

    # Upload track
    result = subprocess.run([
        'curl', '-sk', f'{api}/tracks',
        '-H', f'Authorization: Bearer {token}',
        '-F', f'title={title}',
        '-F', f'artist_id={artist_id}',
        '-F', f'audio=@{filename};type=audio/mpeg'
    ], capture_output=True, text=True)

    try:
        data = json.loads(result.stdout)
        status = data.get('status', '?')
        track_id = data.get('id', '?')[:8]
        print(f'  [{i}/20] {title} - {artist_name} (id={track_id}.. status={status})')
    except:
        print(f'  [{i}/20] FAILED: {title} - {result.stdout[:100]}')
"

echo ""
echo "==> Done! Tracks are queued for transcoding."
echo "    Monitor worker: docker compose logs -f worker"
echo "    Check status:   curl $API/tracks | python3 -m json.tool"
