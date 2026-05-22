import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Trend } from 'k6/metrics';

const errorRate = new Rate('error_rate');
const streamLatency = new Trend('stream_latency', true);

// Set TRACK_ID env var: k6 run -e TRACK_ID=<uuid> k6/load.js
const BASE_URL = __ENV.BASE_URL || 'http://localhost';
const TRACK_ID = __ENV.TRACK_ID || 'replace-with-real-track-uuid';

export const options = {
  stages: [
    { duration: '30s', target: 50  },   // ramp up to 50 VUs
    { duration: '2m',  target: 200 },   // sustained load
    { duration: '30s', target: 500 },   // stress test
    { duration: '30s', target: 0   },   // ramp down
  ],
  thresholds: {
    'http_req_duration':        ['p(99)<200'],   // 99th percentile under 200ms
    'error_rate':               ['rate<0.01'],   // <1% error rate
    'stream_latency':           ['p(95)<500'],   // stream redirect under 500ms
  },
};

export default function () {
  // 80% track metadata reads (typical browse pattern)
  if (Math.random() < 0.8) {
    const res = http.get(`${BASE_URL}/v1/tracks/${TRACK_ID}`, {
      headers: { 'X-Request-ID': `k6-${__VU}-${__ITER}` },
    });
    check(res, { 'track 200': (r) => r.status === 200 });
    errorRate.add(res.status !== 200);
  } else {
    // 20% stream requests
    const res = http.get(`${BASE_URL}/v1/tracks/${TRACK_ID}/stream`, {
      redirects: 0,  // don't follow redirect; measure just the API response
    });
    check(res, { 'stream redirect': (r) => r.status === 307 || r.status === 302 });
    streamLatency.add(res.timings.duration);
    errorRate.add(res.status >= 400);
  }

  sleep(0.1);
}

export function handleSummary(data) {
  return {
    'k6_results/summary.json': JSON.stringify(data, null, 2),
    stdout: JSON.stringify(data.metrics, null, 2),
  };
}
