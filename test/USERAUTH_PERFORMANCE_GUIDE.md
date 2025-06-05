# UserAuth Stress Testing & Performance Optimization

## Overview

This document outlines strategies and recommendations for ensuring the UserAuth module performs efficiently under high-pressure conditions and how to optimize the test suite for thorough performance validation.

## Stress Test Scenarios

### High Concurrency Authentication

The `userauth_stress_test.go` file implements tests for concurrent authentication requests. Key aspects:

```go
func TestConcurrentAuthentication(t *testing.T) {
    // Configure test parameters
    numUsers := 1000
    concurrentRequests := 100
    
    // Set up test environment
    svc := setupTestAuthService(t)
    
    // Create test users
    users := createTestUsers(t, svc, numUsers)
    
    // Run concurrent authentication requests
    var wg sync.WaitGroup
    wg.Add(concurrentRequests)
    
    startTime := time.Now()
    
    for i := 0; i < concurrentRequests; i++ {
        go func(idx int) {
            defer wg.Done()
            
            // Select random user
            userIdx := rand.Intn(numUsers)
            user := users[userIdx]
            
            // Authenticate
            _, _, err := svc.AuthenticateUser(context.Background(), user.Email, "password")
            assert.NoError(t, err)
        }(i)
    }
    
    wg.Wait()
    
    // Calculate metrics
    duration := time.Since(startTime)
    requestsPerSecond := float64(concurrentRequests) / duration.Seconds()
    
    t.Logf("Processed %d requests in %v (%.2f req/sec)", 
        concurrentRequests, duration, requestsPerSecond)
    
    // Assert minimum performance
    assert.GreaterOrEqual(t, requestsPerSecond, float64(50), 
        "Performance below threshold of 50 req/sec")
}
```

### Database Connection Pool Saturation

Tests for database connection pool handling under load:

```go
func TestDatabaseConnectionPoolSaturation(t *testing.T) {
    // Configure large number of concurrent database operations
    concurrentOps := 200
    
    // Set up test environment with configured connection pool
    db, err := setupTestDatabase(t, &gorm.Config{
        PrepareStmt: true,
        SkipDefaultTransaction: true,
        ConnPool: &sql.ConnPool{
            MaxIdleConns: 10,
            MaxOpenConns: 50,
            ConnMaxLifetime: time.Minute,
        },
    })
    
    assert.NoError(t, err)
    svc := setupTestServiceWithDB(t, db)
    
    // Run concurrent database operations
    var wg sync.WaitGroup
    wg.Add(concurrentOps)
    
    for i := 0; i < concurrentOps; i++ {
        go func() {
            defer wg.Done()
            
            // Perform a database operation
            err := svc.PerformDatabaseOperation(context.Background())
            assert.NoError(t, err)
        }()
    }
    
    wg.Wait()
    
    // Check metrics
    stats := db.DB().Stats()
    t.Logf("Max open connections: %d", stats.MaxOpenConnections)
    t.Logf("Open connections: %d", stats.OpenConnections)
    t.Logf("In use connections: %d", stats.InUse)
    t.Logf("Idle connections: %d", stats.Idle)
    
    // Verify connection pool handling
    assert.Less(t, stats.MaxOpenConnections, 51, 
        "Connection pool limit exceeded")
}
```

### Rate Limiter Effectiveness

Tests to verify rate limiting functionality:

```go
func TestRateLimiterEffectiveness(t *testing.T) {
    // Configure test parameters
    requestsPerSecond := 100
    burstSize := 10
    testDuration := 5 * time.Second
    
    // Set up rate limiter
    limiter := auth.NewTieredRateLimiter(requestsPerSecond, burstSize)
    
    // Test rate limiting
    startTime := time.Now()
    endTime := startTime.Add(testDuration)
    
    allowedCount := 0
    rejectedCount := 0
    
    // Send requests as fast as possible
    for time.Now().Before(endTime) {
        allowed := limiter.Allow("test_key")
        if allowed {
            allowedCount++
        } else {
            rejectedCount++
        }
    }
    
    actualDuration := time.Since(startTime)
    expectedMaxAllowed := int(float64(actualDuration.Seconds()) * float64(requestsPerSecond)) + burstSize
    
    t.Logf("Allowed: %d, Rejected: %d, Duration: %v", 
        allowedCount, rejectedCount, actualDuration)
    
    // Verify rate limiter is working correctly
    assert.LessOrEqual(t, allowedCount, expectedMaxAllowed, 
        "Rate limiter allowed too many requests")
}
```

## Performance Optimization Recommendations

### 1. Database Optimizations

- **Connection Pooling**: Configure optimal connection pool settings for high concurrency:
  ```go
  db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
      PrepareStmt: true,
      SkipDefaultTransaction: true,
      ConnPool: &sql.ConnPool{
          MaxIdleConns: 25,
          MaxOpenConns: 100,
          ConnMaxLifetime: time.Minute * 5,
      },
  })
  ```

- **Query Optimizations**:
  - Use appropriate indexes
  - Limit result sets
  - Use specific column selections
  - Avoid N+1 query patterns

### 2. Cache Implementation

- **Token Caching**:
  ```go
  func (s *Service) ValidateToken(ctx context.Context, token string) (*TokenClaims, error) {
      // Check cache first
      if claims, found := s.tokenCache.Get(token); found {
          return claims.(*TokenClaims), nil
      }
      
      // If not in cache, validate and cache result
      claims, err := s.validateTokenSignature(token)
      if err != nil {
          return nil, err
      }
      
      // Cache valid token
      s.tokenCache.Set(token, claims, time.Minute*5)
      return claims, nil
  }
  ```

- **User Data Caching**:
  ```go
  func (s *Service) GetUserByID(ctx context.Context, userID uuid.UUID) (*models.User, error) {
      cacheKey := fmt.Sprintf("user:%s", userID)
      
      // Try cache first
      if userData, found := s.userCache.Get(cacheKey); found {
          return userData.(*models.User), nil
      }
      
      // Fetch from database
      user, err := s.fetchUserFromDB(ctx, userID)
      if err != nil {
          return nil, err
      }
      
      // Cache user data
      s.userCache.Set(cacheKey, user, time.Minute*10)
      return user, nil
  }
  ```

### 3. Concurrency Optimizations

- **Context Management**:
  ```go
  ctx, cancel := context.WithTimeout(parentContext, 3*time.Second)
  defer cancel()
  ```

- **Worker Pools**:
  ```go
  type WorkRequest struct {
      UserID uuid.UUID
      Result chan *models.User
      Error  chan error
  }
  
  func (s *Service) StartWorkerPool(numWorkers int) {
      s.requestQueue = make(chan WorkRequest, 100)
      
      for i := 0; i < numWorkers; i++ {
          go func() {
              for req := range s.requestQueue {
                  user, err := s.GetUserByID(context.Background(), req.UserID)
                  if err != nil {
                      req.Error <- err
                  } else {
                      req.Result <- user
                  }
              }
          }()
      }
  }
  ```

### 4. Memory Management

- **Object Pooling**:
  ```go
  var bufferPool = sync.Pool{
      New: func() interface{} {
          return new(bytes.Buffer)
      },
  }
  
  func getBuffer() *bytes.Buffer {
      return bufferPool.Get().(*bytes.Buffer)
  }
  
  func putBuffer(buf *bytes.Buffer) {
      buf.Reset()
      bufferPool.Put(buf)
  }
  ```

- **Minimize Allocations**:
  ```go
  // Instead of:
  tokens := []string{}
  for i := 0; i < n; i++ {
      tokens = append(tokens, generateToken())
  }
  
  // Use:
  tokens := make([]string, n)
  for i := 0; i < n; i++ {
      tokens[i] = generateToken()
  }
  ```

## Test Suite Optimizations

### 1. Parallel Test Execution

```go
func TestFeatureA(t *testing.T) {
    t.Parallel() // Allow tests to run in parallel
    // Test logic here
}

func TestFeatureB(t *testing.T) {
    t.Parallel()
    // Test logic here
}
```

### 2. Table-Driven Tests

```go
func TestPasswordValidation(t *testing.T) {
    testCases := []struct {
        name           string
        password       string
        expectedResult bool
    }{
        {"Valid password", "StrongP@ss123", true},
        {"Too short", "Short1!", false},
        {"No special chars", "Password123", false},
        {"No numbers", "Password!", false},
        // many more test cases...
    }
    
    for _, tc := range testCases {
        t.Run(tc.name, func(t *testing.T) {
            t.Parallel() // Parallelize subtests
            result := validatePassword(tc.password)
            assert.Equal(t, tc.expectedResult, result)
        })
    }
}
```

### 3. Test Helpers and Fixtures

```go
// Create standardized test users
func createTestUsers(t *testing.T, svc *UserService, count int) []*models.User {
    users := make([]*models.User, count)
    for i := 0; i < count; i++ {
        user, err := svc.RegisterUser(context.Background(), &EnterpriseRegistrationRequest{
            Email:    fmt.Sprintf("user%d@example.com", i),
            Username: fmt.Sprintf("user%d", i),
            Password: "StrongP@ss123",
            // other fields...
        })
        assert.NoError(t, err)
        users[i] = user
    }
    return users
}
```

### 4. Benchmarks

```go
func BenchmarkTokenValidation(b *testing.B) {
    svc := setupTestAuthService(nil)
    token := generateTestToken()
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _, _ = svc.ValidateToken(context.Background(), token)
    }
}

func BenchmarkTokenValidationParallel(b *testing.B) {
    svc := setupTestAuthService(nil)
    token := generateTestToken()
    
    b.ResetTimer()
    b.RunParallel(func(pb *testing.PB) {
        for pb.Next() {
            _, _ = svc.ValidateToken(context.Background(), token)
        }
    })
}
```

## Metrics Collection

### 1. Performance Metrics

```go
func collectPerformanceMetrics(t *testing.T, testName string, fn func() error) {
    startTime := time.Now()
    startAllocs := testing.AllocsPerRun(1, func(){})
    
    err := fn()
    
    duration := time.Since(startTime)
    endAllocs := testing.AllocsPerRun(1, func(){})
    allocsDiff := endAllocs - startAllocs
    
    t.Logf("Test: %s, Duration: %v, Allocations: %.2f", 
        testName, duration, allocsDiff)
    
    // Save metrics to file
    metrics := map[string]interface{}{
        "test":      testName,
        "duration":  duration.Seconds(),
        "allocs":    allocsDiff,
        "timestamp": time.Now().Format(time.RFC3339),
        "error":     err != nil,
    }
    
    saveTestMetrics(t, metrics)
}
```

### 2. Stress Test Report Generation

```go
func generateStressTestReport(t *testing.T, results []TestResult) {
    var buf bytes.Buffer
    
    buf.WriteString("# Stress Test Report\n\n")
    buf.WriteString("| Test | Duration | Req/Sec | Errors | Allocations |\n")
    buf.WriteString("|------|----------|---------|--------|-------------|\n")
    
    for _, result := range results {
        buf.WriteString(fmt.Sprintf("| %s | %v | %.2f | %d | %.2f |\n",
            result.Name, result.Duration, result.RequestsPerSecond,
            result.ErrorCount, result.Allocations))
    }
    
    // Save report
    err := os.WriteFile("test_results/stress_test_report.md", buf.Bytes(), 0644)
    assert.NoError(t, err)
}
```

## Conclusion

By implementing these recommendations, the UserAuth module test suite will effectively validate the system's performance under high-pressure conditions, ensuring it meets production requirements for throughput, reliability, and resource efficiency.
