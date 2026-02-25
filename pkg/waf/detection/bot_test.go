// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package detection

import (
	"fmt"
	"net/http/httptest"
	"testing"
	"time"
)

func TestBotDetector_Detect(t *testing.T) {
	detector := NewBotDetector(false)
	defer detector.Close()

	tests := []struct {
		name        string
		userAgent   string
		wantBot     bool
		wantBotType string
	}{
		// Security scanners - should be detected
		{"nmap", "Nmap Scripting Engine", true, "known_bot"},
		{"nikto", "Nikto/2.1.6", true, "known_bot"},
		{"sqlmap", "sqlmap/1.4.7", true, "known_bot"},
		{"acunetix", "Acunetix Web Vulnerability Scanner", true, "known_bot"},
		{"nessus", "Nessus SOAP", true, "known_bot"},
		{"openvas", "OpenVAS", true, "known_bot"},
		{"burpsuite", "Burp Suite", true, "known_bot"},
		{"owasp zap", "OWASP ZAP/2.10.0", true, "known_bot"},
		{"nuclei", "nuclei/2.5.2", true, "known_bot"},

		// Generic bots
		{"curl", "curl/7.68.0", true, "known_bot"},
		{"wget", "Wget/1.20.3", true, "known_bot"},
		{"python-requests", "python-requests/2.25.1", true, "known_bot"},
		{"python-urllib", "Python-urllib/3.8", true, "known_bot"},
		{"go-http-client", "Go-http-client/1.1", true, "known_bot"},
		{"java", "Java/11.0.11", true, "known_bot"},
		{"httpunit", "HttpUnit/1.7", true, "known_bot"},
		{"httrack", "HTTrack/3.49.2", true, "known_bot"},

		// Headless browsers
		{"headless chrome", "HeadlessChrome/91.0.4472.77", true, "known_bot"},
		{"phantomjs", "PhantomJS/2.1.1", true, "known_bot"},
		{"puppeteer", "puppeteer/10.0.0", true, "known_bot"},
		{"selenium", "Selenium/3.141.0", true, "known_bot"},

		// Data miners
		{"harvester", "Harvester/1.0", true, "known_bot"},
		{"collector", "DataCollector/2.0", true, "known_bot"},

		// Known good bots - should be detected but classified as good
		{"googlebot", "Googlebot/2.1 (+http://www.google.com/bot.html)", true, "good_bot"},
		{"bingbot", "bingbot/2.0 (+http://www.bing.com/bingbot.htm)", true, "good_bot"},
		{"yandexbot", "YandexBot/3.0 (+http://yandex.com/bots)", true, "good_bot"},
		{"duckduckbot", "DuckDuckBot/1.0", true, "good_bot"},
		{"applebot", "Applebot/0.1", true, "good_bot"},
		{"facebookexternalhit", "facebookexternalhit/1.1", true, "good_bot"},
		{"twitterbot", "Twitterbot/1.0", true, "good_bot"},

		// Normal browsers - should NOT be detected
		{"Chrome", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36", false, ""},
		{"Firefox", "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:89.0) Gecko/20100101 Firefox/89.0", false, ""},
		{"Safari", "Mozilla/5.0 (Macintosh; Intel Mac OS X 11_4) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/14.1.1 Safari/605.1.15", false, ""},
		{"Edge", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36 Edg/91.0.864.64", false, ""},
		{"Mobile Chrome", "Mozilla/5.0 (Linux; Android 11; SM-G991B) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.120 Mobile Safari/537.36", false, ""},

		// Empty/suspicious user agents
		{"empty user agent", "", true, "suspicious"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "http://example.com/", nil)
			req.Header.Set("User-Agent", tt.userAgent)
			req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
			req.Header.Set("Accept-Language", "en-US,en;q=0.5")
			req.Header.Set("Accept-Encoding", "gzip, deflate")
			req.Header.Set("Connection", "keep-alive")
			req.RemoteAddr = "1.2.3.4:12345"

			result := detector.Detect(req)

			if result.IsBot != tt.wantBot {
				t.Errorf("Detect() IsBot = %v, want %v (user agent: %q)", result.IsBot, tt.wantBot, tt.userAgent)
			}

			if tt.wantBotType != "" && result.BotType != tt.wantBotType {
				t.Errorf("Detect() BotType = %q, want %q", result.BotType, tt.wantBotType)
			}
		})
	}
}

func TestBotDetector_HeaderAnalysis(t *testing.T) {
	detector := NewBotDetector(false)
	defer detector.Close()

	tests := []struct {
		name          string
		headers       map[string]string
		wantReasons   []string
	}{
		{
			name: "Missing Accept-Language",
			headers: map[string]string{
				"User-Agent":      "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/91.0",
				"Accept-Encoding": "gzip",
			},
			wantReasons: []string{"missing_accept_language"},
		},
		{
			name: "Missing Accept-Encoding",
			headers: map[string]string{
				"User-Agent":      "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/91.0",
				"Accept-Language": "en-US",
			},
			wantReasons: []string{"missing_accept_encoding"},
		},
		{
			name: "Generic Accept header",
			headers: map[string]string{
				"User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/91.0",
				"Accept":     "*/*",
			},
			wantReasons: []string{"generic_accept_header"},
		},
		{
			name: "WebDriver indicator",
			headers: map[string]string{
				"User-Agent": "Mozilla/5.0 webdriver",
			},
			wantReasons: []string{"webdriver_detected"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "http://example.com/", nil)
			for key, value := range tt.headers {
				req.Header.Set(key, value)
			}
			req.RemoteAddr = "1.2.3.4:12345"

			result := detector.Detect(req)

			for _, wantReason := range tt.wantReasons {
				found := false
				for _, reason := range result.Reasons {
					if reason == wantReason {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected reason %q not found in %v", wantReason, result.Reasons)
				}
			}
		})
	}
}

func TestBotDetector_BehaviorTracking(t *testing.T) {
	detector := NewBotDetector(false)
	defer detector.Close()

	clientIP := "10.0.0.1"

	// Simulate rapid requests
	for i := 0; i < 20; i++ {
		req := httptest.NewRequest("GET", fmt.Sprintf("http://example.com/page%d", i), nil)
		req.Header.Set("User-Agent", "Mozilla/5.0 Chrome/91.0")
		req.Header.Set("Accept-Language", "en-US")
		req.Header.Set("Accept-Encoding", "gzip")
		req.RemoteAddr = clientIP + ":12345"

		detector.Detect(req)
	}

	// Check behavior stats via exported method
	stats := detector.GetBehaviorStats()
	trackedClients, ok := stats["tracked_clients"].(int)
	if !ok || trackedClients < 1 {
		t.Error("Expected at least 1 tracked client")
	}
}

func TestBotDetector_IsGoodBot(t *testing.T) {
	detector := NewBotDetector(false)
	defer detector.Close()

	goodBots := []string{
		"Googlebot/2.1",
		"bingbot/2.0",
		"YandexBot/3.0",
		"DuckDuckBot/1.0",
		"facebookexternalhit/1.1",
		"Twitterbot/1.0",
		"LinkedInBot/1.0",
		"Applebot/0.1",
	}

	for _, ua := range goodBots {
		req := httptest.NewRequest("GET", "http://example.com/", nil)
		req.Header.Set("User-Agent", ua)
		req.RemoteAddr = "1.2.3.4:12345"

		if !detector.IsGoodBot(req) {
			t.Errorf("IsGoodBot() = false for %q, want true", ua)
		}
	}
}

func TestBotDetector_DetectWithDetails(t *testing.T) {
	detector := NewBotDetector(false)
	defer detector.Close()

	tests := []struct {
		name         string
		userAgent    string
		wantDetected bool
		wantScoreMin int
	}{
		{
			name:         "Security scanner",
			userAgent:    "sqlmap/1.4.7",
			wantDetected: true,
			wantScoreMin: 50,
		},
		{
			name:         "Normal browser",
			userAgent:    "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/91.0.4472.124",
			wantDetected: false,
			wantScoreMin: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "http://example.com/", nil)
			req.Header.Set("User-Agent", tt.userAgent)
			req.Header.Set("Accept-Language", "en-US,en;q=0.9")
			req.Header.Set("Accept-Encoding", "gzip, deflate, br")
			req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
			req.Header.Set("Connection", "keep-alive")
			req.Header.Set("Sec-Fetch-Dest", "document")
			req.Header.Set("Sec-Fetch-Mode", "navigate")
			req.Header.Set("Sec-Fetch-Site", "none")
			req.Header.Set("Sec-Fetch-User", "?1")
			req.RemoteAddr = "1.2.3.4:12345"

			result := detector.DetectWithDetails(req)

			// For security scanners, check that they are detected
			if tt.wantDetected && !result.Detected {
				t.Errorf("DetectWithDetails().Detected = %v, want %v", result.Detected, tt.wantDetected)
			}

			if result.Score < tt.wantScoreMin {
				t.Errorf("DetectWithDetails().Score = %d, want >= %d", result.Score, tt.wantScoreMin)
			}
		})
	}
}

func TestBotDetector_AddKnownGoodBot(t *testing.T) {
	detector := NewBotDetector(false)
	defer detector.Close()

	// Add a custom good bot
	detector.AddKnownGoodBot("mycompanybot")

	req := httptest.NewRequest("GET", "http://example.com/", nil)
	req.Header.Set("User-Agent", "MyCompanyBot/1.0")
	req.RemoteAddr = "1.2.3.4:12345"

	if !detector.IsGoodBot(req) {
		t.Error("Expected custom bot to be recognized as good bot")
	}
}

func TestBotDetector_AddBotPattern(t *testing.T) {
	detector := NewBotDetector(false)
	defer detector.Close()

	// Add a custom pattern
	err := detector.AddBotPattern(`(?i)customscanner`)
	if err != nil {
		t.Fatalf("AddBotPattern failed: %v", err)
	}

	req := httptest.NewRequest("GET", "http://example.com/", nil)
	req.Header.Set("User-Agent", "CustomScanner/1.0")
	req.Header.Set("Accept-Language", "en-US")
	req.Header.Set("Accept-Encoding", "gzip")
	req.RemoteAddr = "1.2.3.4:12345"

	result := detector.Detect(req)
	if !result.IsBot {
		t.Error("Expected custom pattern to be detected")
	}
}

func TestBotDetector_SuspiciousPath(t *testing.T) {
	detector := NewBotDetector(false)
	defer detector.Close()

	sensitiveURLs := []string{
		"http://example.com/admin",
		"http://example.com/wp-admin/",
		"http://example.com/phpmyadmin/",
		"http://example.com/.git/config",
		"http://example.com/.env",
	}

	for _, url := range sensitiveURLs {
		req := httptest.NewRequest("GET", url, nil)
		req.Header.Set("User-Agent", "Mozilla/5.0 Chrome/91.0")
		req.RemoteAddr = "1.2.3.4:12345"

		result := detector.Detect(req)

		hasReason := false
		for _, reason := range result.Reasons {
			if reason == "sensitive_path_access" {
				hasReason = true
				break
			}
		}

		if !hasReason {
			t.Errorf("Expected 'sensitive_path_access' reason for URL %q", url)
		}
	}
}

func TestBehaviorTracker(t *testing.T) {
	tracker := NewBehaviorTracker(time.Hour, 1000)
	defer tracker.Close()

	// Track multiple requests
	for i := 0; i < 10; i++ {
		req := httptest.NewRequest("GET", "http://example.com/page", nil)
		tracker.Track("test-client", req)
	}

	behavior := tracker.GetBehavior("test-client")
	if behavior == nil {
		t.Fatal("Expected behavior to be tracked")
	}

	if behavior.RequestCount != 10 {
		t.Errorf("RequestCount = %d, want 10", behavior.RequestCount)
	}
}

func TestBehaviorTracker_RecordStatusCode(t *testing.T) {
	tracker := NewBehaviorTracker(time.Hour, 1000)
	defer tracker.Close()

	// Track a request first
	req := httptest.NewRequest("GET", "http://example.com/", nil)
	tracker.Track("test-client", req)

	// Record status codes
	tracker.RecordStatusCode("test-client", 200)
	tracker.RecordStatusCode("test-client", 200)
	tracker.RecordStatusCode("test-client", 404)
	tracker.RecordStatusCode("test-client", 500)

	behavior := tracker.GetBehavior("test-client")
	if behavior == nil {
		t.Fatal("Expected behavior to be tracked")
	}

	if behavior.StatusCodes[200] != 2 {
		t.Errorf("StatusCodes[200] = %d, want 2", behavior.StatusCodes[200])
	}

	// Check error rate calculation
	if behavior.ErrorRate == 0 {
		t.Error("Expected error rate to be calculated")
	}
}

func TestFingerprintStore(t *testing.T) {
	store := NewFingerprintStore()

	fp := &BrowserFingerprint{
		Hash:           "test-hash",
		UserAgent:      "Mozilla/5.0 Chrome/91.0",
		AcceptLanguage: "en-US",
		AcceptEncoding: "gzip",
	}

	store.Store(fp)

	retrieved := store.Get("test-hash")
	if retrieved == nil {
		t.Fatal("Expected fingerprint to be retrieved")
	}

	if retrieved.UserAgent != fp.UserAgent {
		t.Errorf("UserAgent = %q, want %q", retrieved.UserAgent, fp.UserAgent)
	}

	// Store again - should update count (increments from 0 to 1)
	store.Store(fp)
	retrieved = store.Get("test-hash")
	if retrieved.SeenCount != 1 {
		t.Errorf("SeenCount = %d, want 1", retrieved.SeenCount)
	}

	// Store a third time - should increment to 2
	store.Store(fp)
	retrieved = store.Get("test-hash")
	if retrieved.SeenCount != 2 {
		t.Errorf("SeenCount = %d, want 2", retrieved.SeenCount)
	}
}

func TestBotDetector_GetBehaviorStats(t *testing.T) {
	detector := NewBotDetector(false)
	defer detector.Close()

	// Generate some activity
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("GET", "http://example.com/", nil)
		req.Header.Set("User-Agent", "Mozilla/5.0")
		req.RemoteAddr = "1.2.3." + string(rune(i+48)) + ":12345"
		detector.Detect(req)
	}

	stats := detector.GetBehaviorStats()

	if _, ok := stats["tracked_clients"]; !ok {
		t.Error("Expected 'tracked_clients' in stats")
	}

	if _, ok := stats["total_requests"]; !ok {
		t.Error("Expected 'total_requests' in stats")
	}
}

func TestBotDetector_SetOptions(t *testing.T) {
	detector := NewBotDetector(false)
	defer detector.Close()

	// Test SetEnableFingerprinting
	detector.SetEnableFingerprinting(false)
	detector.SetEnableFingerprinting(true)

	// Test SetEnableBehavior
	detector.SetEnableBehavior(false)
	detector.SetEnableBehavior(true)

	// Should not panic
}

func TestIsRegularTiming(t *testing.T) {
	tests := []struct {
		name    string
		timings []int64
		want    bool
	}{
		{
			name:    "Regular timing",
			timings: []int64{100, 100, 100, 100, 100},
			want:    true,
		},
		{
			name:    "Irregular timing",
			timings: []int64{100, 500, 50, 1000, 200},
			want:    false,
		},
		{
			name:    "Too few samples",
			timings: []int64{100, 100},
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isRegularTiming(tt.timings)
			if result != tt.want {
				t.Errorf("isRegularTiming(%v) = %v, want %v", tt.timings, result, tt.want)
			}
		})
	}
}

func TestGetClientIP(t *testing.T) {
	tests := []struct {
		name        string
		remoteAddr  string
		xff         string
		xri         string
		expectedIP  string
	}{
		{
			name:       "RemoteAddr only",
			remoteAddr: "1.2.3.4:12345",
			expectedIP: "1.2.3.4:12345",
		},
		{
			name:       "X-Forwarded-For single",
			remoteAddr: "127.0.0.1:12345",
			xff:        "1.2.3.4",
			expectedIP: "1.2.3.4",
		},
		{
			name:       "X-Forwarded-For multiple",
			remoteAddr: "127.0.0.1:12345",
			xff:        "1.2.3.4, 5.6.7.8, 9.10.11.12",
			expectedIP: "1.2.3.4",
		},
		{
			name:       "X-Real-IP",
			remoteAddr: "127.0.0.1:12345",
			xri:        "1.2.3.4",
			expectedIP: "1.2.3.4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "http://example.com/", nil)
			req.RemoteAddr = tt.remoteAddr
			if tt.xff != "" {
				req.Header.Set("X-Forwarded-For", tt.xff)
			}
			if tt.xri != "" {
				req.Header.Set("X-Real-IP", tt.xri)
			}

			ip := getClientIP(req)
			if ip != tt.expectedIP {
				t.Errorf("getClientIP() = %q, want %q", ip, tt.expectedIP)
			}
		})
	}
}

func BenchmarkBotDetector_Detect(b *testing.B) {
	detector := NewBotDetector(false)
	defer detector.Close()

	req := httptest.NewRequest("GET", "http://example.com/page", nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/91.0.4472.124 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	req.RemoteAddr = "1.2.3.4:12345"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		detector.Detect(req)
	}
}

func BenchmarkBotDetector_DetectBot(b *testing.B) {
	detector := NewBotDetector(false)
	defer detector.Close()

	req := httptest.NewRequest("GET", "http://example.com/page", nil)
	req.Header.Set("User-Agent", "sqlmap/1.4.7")
	req.RemoteAddr = "1.2.3.4:12345"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		detector.Detect(req)
	}
}

func BenchmarkBehaviorTracker_Track(b *testing.B) {
	tracker := NewBehaviorTracker(time.Hour, 100000)
	defer tracker.Close()

	req := httptest.NewRequest("GET", "http://example.com/page", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tracker.Track("test-client", req)
	}
}
