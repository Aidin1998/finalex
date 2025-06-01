import http from 'k6/http';
import { check, sleep } from 'k6';

export let options = {
    vus: 10000, // virtual users
    duration: '12h',
    rps: 100000, // requests per second
    thresholds: {
        http_req_duration: ['p(99)<50', 'avg<15'],
        http_req_failed: ['rate<0.005'],
    },
};

export default function () {
    let res = http.post('http://localhost:8080/api/v1/order', JSON.stringify({
        symbol: 'BTC-USD',
        side: 'buy',
        price: Math.random() * 100000,
        quantity: Math.random() * 2 + 0.01,
        type: 'limit',
        user_id: Math.floor(Math.random() * 1000000),
    }), {
        headers: { 'Content-Type': 'application/json' },
    });
    check(res, {
        'status is 200': (r) => r.status === 200,
        'latency < 50ms': (r) => r.timings.duration < 50,
    });
    sleep(0.01); // ~100 TPS per VU
}
