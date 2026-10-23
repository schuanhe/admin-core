package auth

import (
	"sync"
	"testing"
	"time"
)

func TestRateLimiterConcurrencyAndExpiry(t *testing.T) {
	m := New(nil, 12)
	var wg sync.WaitGroup
	allowed := make(chan bool, 100)
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); allowed <- m.Allow("127.0.0.1") }()
	}
	wg.Wait()
	close(allowed)
	count := 0
	for yes := range allowed {
		if yes {
			count++
		}
	}
	if count != 10 {
		t.Fatalf("并发限速应只允许10次，实际%d", count)
	}
	m.mu.Lock()
	m.attempts["127.0.0.1"] = bucket{count: 10, until: time.Now().Add(-time.Second)}
	m.mu.Unlock()
	if !m.Allow("127.0.0.1") {
		t.Fatal("过期窗口应恢复尝试")
	}
}
func TestSecretEqualityAndTokens(t *testing.T) {
	if !Equal("secret-a", "secret-a") || Equal("secret-a", "secret-b") {
		t.Fatal("密钥比较错误")
	}
	a, e := token()
	if e != nil {
		t.Fatal(e)
	}
	b, e := token()
	if e != nil {
		t.Fatal(e)
	}
	if len(a) != 64 || a == b || Hash(a) == a {
		t.Fatal("安全令牌生成或存储哈希错误")
	}
}
