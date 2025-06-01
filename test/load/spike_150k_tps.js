import http from 'k6/http';
import { check, sleep } from 'k6';

export let options = {
    vus: 15000, // virtual users
    stages: [
        { duration: '2m', target: 15000 }, // ramp up
        { duration: '30m', target: 15000 }, // spike
        { duration: '2m', target: 0 },      // ramp down
    ],
    rps: 150000, // requests per second
    thresholds: {
        http_req_duration: ['p(99)<80', 'avg<25'],
        http_req_failed: ['rate<0.01'],
    },
};

export default function () {
    let res = http.post('http://localhost:8080/api/v1/order', JSON.stringify({
        symbol: 'BTC-USD',
        side: 'sell',
        price: Math.random() * 100000,
        quantity: Math.random() * 2 + 0.01,
        type: 'limit',
        user_id: Math.floor(Math.random() * 1000000),
    }), {
        headers: { 'Content-Type': 'application/json' },
    });
    check(res, {
        'status is 200': (r) => r.status === 200,
        'latency < 80ms': (r) => r.timings.duration < 80,
    });
    sleep(0.01);
}
