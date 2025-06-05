import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Trend, Counter } from 'k6/metrics';

// Custom metrics for detailed performance analysis
const errorRate = new Rate('errors');
const responseTime = new Trend('response_time');
const throughput = new Counter('requests');

// Test configuration for ultra-high concurrency
export const options = {
  stages: [
    // Ramp up to 100k RPS over 30 seconds
    { duration: '30s', target: 1000 },   // Warm up
    { duration: '60s', target: 5000 },   // Moderate load
    { duration: '60s', target: 10000 },  // High load
    { duration: '120s', target: 25000 }, // Very high load
    { duration: '180s', target: 50000 }, // Ultra high load
    { duration: '300s', target: 100000 }, // Target: 100k RPS
    { duration: '60s', target: 0 },      // Ramp down
  ],
  thresholds: {
    // Performance requirements
    'http_req_duration': ['p(95)<10'],      // 95% under 10ms
    'http_req_duration{operation:balance_query}': ['p(95)<5'],   // Balance queries under 5ms
    'http_req_duration{operation:balance_update}': ['p(95)<10'], // Balance updates under 10ms
    'http_req_failed': ['rate<0.001'],     // Error rate under 0.1%
    'errors': ['rate<0.001'],
    'requests': ['count>10000000'],        // At least 10M requests total
  },
  summaryTrendStats: ['min', 'med', 'avg', 'p(90)', 'p(95)', 'p(99)', 'p(99.9)', 'max'],
  summaryTimeUnit: 'ms',
};

// Base URL for the accounts service
const BASE_URL = 'http://accounts-app:8080';

// Test data generators
function generateUserId() {
  return `user-${Math.floor(Math.random() * 100000)}`;
}

function generateCurrency() {
  const currencies = ['USD', 'EUR', 'BTC', 'ETH', 'USDT'];
  return currencies[Math.floor(Math.random() * currencies.length)];
}

function generateAmount() {
  return (Math.random() * 10000).toFixed(8);
}

// Test scenarios for different operations
export default function() {
  const userId = generateUserId();
  const currency = generateCurrency();
  const amount = generateAmount();
  
  // Weighted test scenarios based on real-world usage patterns
  const scenario = Math.random();
  
  if (scenario < 0.6) {
    // 60% - Balance queries (most frequent operation)
    testBalanceQuery(userId, currency);
  } else if (scenario < 0.8) {
    // 20% - Balance updates
    testBalanceUpdate(userId, currency, amount);
  } else if (scenario < 0.9) {
    // 10% - Reservation operations
    testReservation(userId, currency, amount);
  } else {
    // 10% - Account creation and complex operations
    testAccountManagement(userId, currency);
  }
  
  // Small sleep to simulate realistic user behavior
  sleep(0.001); // 1ms think time
}

function testBalanceQuery(userId, currency) {
  const start = Date.now();
  
  const response = http.get(`${BASE_URL}/api/v1/accounts/balance`, {
    params: {
      user_id: userId,
      currency: currency,
    },
    tags: {
      operation: 'balance_query',
      currency: currency,
    },
  });
  
  const duration = Date.now() - start;
  responseTime.add(duration);
  throughput.add(1);
  
  const success = check(response, {
    'balance query status is 200': (r) => r.status === 200,
    'balance query response time < 5ms': () => duration < 5,
    'balance query has valid response': (r) => {
      try {
        const body = JSON.parse(r.body);
        return body.balance !== undefined && body.available_balance !== undefined;
      } catch {
        return false;
      }
    },
  });
  
  if (!success) {
    errorRate.add(1);
  }
}

function testBalanceUpdate(userId, currency, amount) {
  const start = Date.now();
  
  const payload = {
    user_id: userId,
    currency: currency,
    amount: parseFloat(amount),
    operation_type: 'credit',
    reference_id: `ref-${Date.now()}-${Math.random()}`,
  };
  
  const response = http.post(`${BASE_URL}/api/v1/accounts/balance/update`, 
    JSON.stringify(payload), 
    {
      headers: {
        'Content-Type': 'application/json',
      },
      tags: {
        operation: 'balance_update',
        currency: currency,
      },
    }
  );
  
  const duration = Date.now() - start;
  responseTime.add(duration);
  throughput.add(1);
  
  const success = check(response, {
    'balance update status is 200 or 409': (r) => r.status === 200 || r.status === 409, // 409 for conflicts
    'balance update response time < 10ms': () => duration < 10,
    'balance update has valid response': (r) => {
      try {
        const body = JSON.parse(r.body);
        return body.success !== undefined || body.error !== undefined;
      } catch {
        return false;
      }
    },
  });
  
  if (!success) {
    errorRate.add(1);
  }
}

function testReservation(userId, currency, amount) {
  const start = Date.now();
  
  const payload = {
    user_id: userId,
    currency: currency,
    amount: parseFloat(amount),
    reservation_type: 'trade',
    reference_id: `reservation-${Date.now()}-${Math.random()}`,
  };
  
  const response = http.post(`${BASE_URL}/api/v1/accounts/reserve`, 
    JSON.stringify(payload), 
    {
      headers: {
        'Content-Type': 'application/json',
      },
      tags: {
        operation: 'reservation',
        currency: currency,
      },
    }
  );
  
  const duration = Date.now() - start;
  responseTime.add(duration);
  throughput.add(1);
  
  const success = check(response, {
    'reservation status is 200 or 400': (r) => r.status === 200 || r.status === 400, // 400 for insufficient funds
    'reservation response time < 15ms': () => duration < 15,
    'reservation has valid response': (r) => {
      try {
        const body = JSON.parse(r.body);
        return body.reservation_id !== undefined || body.error !== undefined;
      } catch {
        return false;
      }
    },
  });
  
  if (!success) {
    errorRate.add(1);
  }
}

function testAccountManagement(userId, currency) {
  const start = Date.now();
  
  const payload = {
    user_id: userId,
    currency: currency,
    account_type: 'spot',
  };
  
  const response = http.post(`${BASE_URL}/api/v1/accounts/create`, 
    JSON.stringify(payload), 
    {
      headers: {
        'Content-Type': 'application/json',
      },
      tags: {
        operation: 'account_create',
        currency: currency,
      },
    }
  );
  
  const duration = Date.now() - start;
  responseTime.add(duration);
  throughput.add(1);
  
  const success = check(response, {
    'account create status is 200 or 409': (r) => r.status === 200 || r.status === 409, // 409 for existing
    'account create response time < 20ms': () => duration < 20,
    'account create has valid response': (r) => {
      try {
        const body = JSON.parse(r.body);
        return body.account_id !== undefined || body.error !== undefined;
      } catch {
        return false;
      }
    },
  });
  
  if (!success) {
    errorRate.add(1);
  }
}

// Performance test for sustained load
export function sustainedLoadTest() {
  console.log('Starting sustained load test for 100k+ RPS...');
  
  // Run for 10 minutes at maximum load
  const testDuration = 600; // 10 minutes
  const startTime = Date.now();
  
  while ((Date.now() - startTime) / 1000 < testDuration) {
    // Execute multiple operations in parallel
    for (let i = 0; i < 10; i++) {
      default();
    }
  }
  
  console.log('Sustained load test completed');
}

// Stress test for breaking point analysis
export function stressTest() {
  console.log('Starting stress test to find breaking point...');
  
  // Gradually increase load until system breaks
  const maxVUsers = 200000; // Maximum virtual users
  const increment = 1000;
  
  for (let vusers = 1000; vusers <= maxVUsers; vusers += increment) {
    console.log(`Testing with ${vusers} virtual users...`);
    
    // Run test for 30 seconds at this load level
    const testStart = Date.now();
    while ((Date.now() - testStart) < 30000) {
      default();
    }
    
    // Check if error rate is acceptable
    if (errorRate.rate > 0.05) { // More than 5% errors
      console.log(`Breaking point reached at ${vusers} virtual users`);
      break;
    }
  }
  
  console.log('Stress test completed');
}

// Teardown function for cleanup
export function teardown(data) {
  console.log('Performance test completed');
  console.log(`Total requests: ${throughput.count}`);
  console.log(`Error rate: ${(errorRate.rate * 100).toFixed(2)}%`);
  console.log(`Average response time: ${responseTime.avg.toFixed(2)}ms`);
  console.log(`95th percentile: ${responseTime.p95.toFixed(2)}ms`);
  console.log(`99th percentile: ${responseTime.p99.toFixed(2)}ms`);
}
