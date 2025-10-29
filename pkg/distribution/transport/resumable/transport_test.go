package resumable

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	testutil "github.com/docker/model-runner/pkg/distribution/transport/internal/testing"
)

// blockingBody simulates a response body that blocks on Read until closed.
type blockingBody struct {
	ch chan struct{}
}

func newBlockingBody() *blockingBody               { return &blockingBody{ch: make(chan struct{})} }
func (b *blockingBody) Read(p []byte) (int, error) { <-b.ch; return 0, io.EOF }
func (b *blockingBody) Close() error               { close(b.ch); return nil }

// TestResumeSingleFailure_Succeeds tests resuming after a single failure.
func TestResumeSingleFailure_Succeeds(t *testing.T) {
	url := "https://example.com/test-file"
	payload := testutil.GenerateTestData(5000)

	ft := testutil.NewFakeTransport()
	ft.Add(url, &testutil.FakeResource{
		Data:          bytes.NewReader(payload),
		Length:        int64(len(payload)),
		SupportsRange: true,
		ETag:          `"test-etag"`,
	})

	// Simulate failure after 2500 bytes on first request.
	ft.SetFailAfter(url, 2500)

	client := &http.Client{
		Transport: New(ft, WithMaxRetries(3)),
	}

	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	defer resp.Body.Close()

	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}

	testutil.AssertDataEquals(t, got, payload)

	// Verify resume happened.
	reqs := ft.GetRequests()
	var rangeRequests int
	for _, req := range reqs {
		if req.Header.Get("Range") != "" {
			rangeRequests++
			t.Logf("Range request: %s", req.Header.Get("Range"))
		}
	}

	if rangeRequests < 1 {
		t.Error("expected at least one range request for resume")
	}
}

// TestResumeMultipleFailuresWithinBudget_Succeeds tests multiple resume
// attempts.
func TestResumeMultipleFailuresWithinBudget_Succeeds(t *testing.T) {
	url := "https://example.com/multi-fail"
	payload := testutil.GenerateTestData(10000)

	ft := testutil.NewFakeTransport()

	ft.Add(url, &testutil.FakeResource{
		Data:          bytes.NewReader(payload),
		Length:        int64(len(payload)),
		SupportsRange: true,
		ETag:          `"multi-fail-etag"`,
	})

	// Hook to inject failures - use SetFailAfter multiple times.
	failurePoints := []int{2000, 5000, 7500}
	failureIndex := 0
	requestCount := 0
	var mu sync.Mutex
	ft.ResponseHook = func(resp *http.Response) {
		if resp.Request.Method == http.MethodGet &&
			failureIndex < len(failurePoints) {
			// For non-range requests, inject failure.
			if resp.Request.Header.Get("Range") == "" {
				mu.Lock()
				idx := failureIndex
				failureIndex++
				mu.Unlock()
				resp.Body = testutil.NewFlakyReader(
					bytes.NewReader(payload),
					int64(len(payload)),
					failurePoints[idx],
				)
			} else {
				// For range requests, check which failure point we're at.
				mu.Lock()
				requestCount++
				rc := requestCount
				fi := failureIndex
				mu.Unlock()
				if rc <= len(failurePoints) &&
					fi < len(failurePoints) {
					// Parse range to determine data slice.
					rangeHeader := resp.Request.Header.Get("Range")
					if rangeHeader != "" {
						// Simple parsing for bytes=N- format.
						var start int
						fmt.Sscanf(rangeHeader, "bytes=%d-", &start)
						rangeData := payload[start:]

						// Apply next failure point relative to this
						// range.
						nextFailure := failurePoints[fi] - start
						if nextFailure > 0 &&
							nextFailure < len(rangeData) {
							resp.Body = testutil.NewFlakyReader(
								bytes.NewReader(rangeData),
								int64(len(rangeData)),
								nextFailure,
							)
							mu.Lock()
							failureIndex++
							mu.Unlock()
						}
					}
				}
			}
		}
	}

	client := &http.Client{
		Transport: New(ft, WithMaxRetries(5)),
	}

	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	defer resp.Body.Close()

	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}

	testutil.AssertDataEquals(t, got, payload)

	// Check that multiple resumes happened.
	reqs := ft.GetRequests()
	var rangeCount int
	for _, req := range reqs {
		if req.Header.Get("Range") != "" {
			rangeCount++
		}
	}

	if rangeCount < 2 {
		t.Errorf("expected at least 2 range requests, got %d", rangeCount)
	}
}

// TestExceedRetryBudget_Fails tests failure when retry budget is exceeded.
func TestExceedRetryBudget_Fails(t *testing.T) {
	url := "https://example.com/too-many-failures"
	payload := testutil.GenerateTestData(4096)

	ft := testutil.NewFakeTransport()
	ft.Add(url, &testutil.FakeResource{
		Data:          bytes.NewReader(payload),
		Length:        int64(len(payload)),
		SupportsRange: true,
		ETag:          `"fail-test"`,
	})

	// Always fail after 100 bytes.
	ft.ResponseHook = func(resp *http.Response) {
		if resp.Request.Method == http.MethodGet {
			resp.Body = testutil.NewFlakyReader(bytes.NewReader(payload), int64(len(payload)), 100)
		}
	}

	client := &http.Client{
		Transport: New(ft, WithMaxRetries(2)), // Low retry limit.
	}

	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	defer resp.Body.Close()

	_, err = io.ReadAll(resp.Body)
	if err == nil {
		t.Error("expected error after exceeding retry budget")
	}

	// Check that retries were attempted.
	reqs := ft.GetRequests()
	var attempts int
	for _, req := range reqs {
		if req.Method == http.MethodGet {
			attempts++
		}
	}

	// Initial + 2 retries = 3 total.
	if attempts < 2 {
		t.Errorf("expected at least 2 GET attempts, got %d", attempts)
	}
}

// TestReadCloseInterleaving ensures Close does not deadlock with a blocked Read
// and unblocks promptly.
func TestReadCloseInterleaving(t *testing.T) {
	url := "https://example.com/blocking"
	payload := testutil.GenerateTestData(1024)

	ft := testutil.NewFakeTransport()
	ft.Add(url, &testutil.FakeResource{
		Data:          bytes.NewReader(payload),
		Length:        int64(len(payload)),
		SupportsRange: true,
		ETag:          `"etag"`,
	})
	// Replace body with a blocking body for the initial GET.
	bb := newBlockingBody()
	ft.ResponseHook = func(resp *http.Response) {
		if resp.Request.Method == http.MethodGet && resp.Request.Header.Get("Range") == "" {
			resp.Body = bb
		}
	}

	client := &http.Client{Transport: New(ft)}
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = io.ReadAll(resp.Body)
	}()

	// Close should unblock the read goroutine promptly.
	if err := resp.Body.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("read did not unblock after Close")
	}
}

// TestMultiRange_PassThrough ensures multi-range requests are not wrapped.
func TestMultiRange_PassThrough(t *testing.T) {
	url := "https://example.com/multirange"
	payload := testutil.GenerateTestData(4096)

	ft := testutil.NewFakeTransport()
	ft.Add(url, &testutil.FakeResource{
		Data:          bytes.NewReader(payload),
		Length:        int64(len(payload)),
		SupportsRange: true,
	})

	client := &http.Client{Transport: New(ft)}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Range", "bytes=0-10,20-30")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	// FakeTransport does not implement multi-range; it returns 400.
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 from fake transport for multi-range, got %d", resp.StatusCode)
	}

	// Ensure no If-Range was injected on request headers.
	hdrs := ft.GetRequestHeaders(url)
	for _, h := range hdrs {
		if h.Get("If-Range") != "" {
			t.Error("unexpected If-Range header on multi-range request")
		}
	}
}

// TestInitialRange_200OK_Ignored ensures if server responds 200 to a ranged
// request, the stream is treated as starting at 0 and reads succeed.
func TestInitialRange_200OK_Ignored(t *testing.T) {
	url := "https://example.com/range-ignored"
	payload := testutil.GenerateTestData(2048)

	ft := testutil.NewFakeTransport()
	ft.Add(url, &testutil.FakeResource{
		Data:          bytes.NewReader(payload),
		Length:        int64(len(payload)),
		SupportsRange: true,
		ETag:          `"e"`,
	})

	// Force 200 full response even when Range is present.
	ft.ResponseHook = func(resp *http.Response) {
		if resp.Request.Header.Get("Range") != "" && resp.StatusCode == http.StatusPartialContent {
			resp.StatusCode = http.StatusOK
			resp.Status = "200 OK"
			resp.Header.Del("Content-Range")
			resp.Body = io.NopCloser(bytes.NewReader(payload))
		}
	}

	client := &http.Client{Transport: New(ft)}
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Range", "bytes=100-199")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	testutil.AssertDataEquals(t, data, payload)
}

// TestRedirectOnResume returns 3xx for resume request and expects a clear error.
func TestRedirectOnResume(t *testing.T) {
	url := "https://example.com/redirect-on-resume"
	payload := testutil.GenerateTestData(5000)
	etag := `"strong"`

	ft := testutil.NewFakeTransport()
	ft.Add(url, &testutil.FakeResource{
		Data:          bytes.NewReader(payload),
		Length:        int64(len(payload)),
		SupportsRange: true,
		ETag:          etag,
	})
	ft.SetFailAfter(url, 2500)

	ft.ResponseHook = func(resp *http.Response) {
		if resp.Request.Header.Get("Range") != "" {
			resp.StatusCode = http.StatusFound
			resp.Status = "302 Found"
			resp.Header.Del("Content-Range")
			resp.Body = io.NopCloser(bytes.NewReader(nil))
		}
	}

	client := &http.Client{Transport: New(ft, WithMaxRetries(2))}
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	_, err = io.ReadAll(resp.Body)
	if err == nil || !strings.Contains(err.Error(), "redirect status") {
		t.Fatalf("expected redirect error, got %v", err)
	}
}

// TestWrongStartOnResume_IsRejected tests handling of unexpected range
// responses.
func TestWrongStartOnResume_IsRejected(t *testing.T) {
	url := "https://example.com/wrong-start"
	payload := testutil.GenerateTestData(5000)

	ft := testutil.NewFakeTransport()
	ft.Add(url, &testutil.FakeResource{
		Data:          bytes.NewReader(payload),
		Length:        int64(len(payload)),
		SupportsRange: true,
		ETag:          `"test"`,
	})

	// Return wrong range on resume.
	resumeAttempted := false
	var muResume sync.Mutex
	ft.ResponseHook = func(resp *http.Response) {
		if resp.Request.Header.Get("Range") == "bytes=2500-" {
			muResume.Lock()
			resumeAttempted = true
			muResume.Unlock()
			// Return wrong start position.
			resp.Header.Set("Content-Range", "bytes 3000-4999/5000")
			resp.Body = io.NopCloser(testutil.NewFlakyReader(
				bytes.NewReader(payload[3000:]),
				int64(len(payload[3000:])),
				0,
			))
		}
	}

	// First fail after 2500 bytes.
	ft.SetFailAfter(url, 2500)

	client := &http.Client{
		Transport: New(ft, WithMaxRetries(3)),
	}

	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	_, err = io.ReadAll(resp.Body)
	if err == nil {
		t.Error("expected error due to wrong range start")
	}

	muResume.Lock()
	attempted := resumeAttempted
	muResume.Unlock()
	if !attempted {
		t.Error("resume was not attempted")
	}
}

// TestNon206OnResume_IsRejected tests handling when server returns 200
// instead of 206.
func TestNon206OnResume_IsRejected(t *testing.T) {
	url := "https://example.com/non-206"
	payload := testutil.GenerateTestData(5000)

	ft := testutil.NewFakeTransport()
	ft.Add(url, &testutil.FakeResource{
		Data:          bytes.NewReader(payload),
		Length:        int64(len(payload)),
		SupportsRange: true,
		ETag:          `"test"`,
	})

	// Return 200 on range request (simulating resource change).
	ft.ResponseHook = func(resp *http.Response) {
		if resp.Request.Header.Get("Range") == "bytes=2500-" {
			resp.StatusCode = http.StatusOK
			resp.Status = "200 OK"
			resp.Header.Del("Content-Range")
			resp.Body = io.NopCloser(testutil.NewFlakyReader(
				bytes.NewReader(payload),
				int64(len(payload)),
				0,
			))
		}
	}

	ft.SetFailAfter(url, 2500)

	client := &http.Client{
		Transport: New(ft, WithMaxRetries(3)),
	}

	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	_, err = io.ReadAll(resp.Body)
	if err == nil ||
		err.Error() != "resumable: server returned 200 to a range request; resource may have changed" {
		t.Errorf("expected specific error, got: %v", err)
	}
}

// TestNoRangeSupport_PassesThrough_NoResume tests fallback when server
// doesn't support ranges.
func TestNoRangeSupport_PassesThrough_NoResume(t *testing.T) {
	url := "https://example.com/no-range"
	payload := testutil.GenerateTestData(5000)

	ft := testutil.NewFakeTransport()
	ft.Add(url, &testutil.FakeResource{
		Data:          bytes.NewReader(payload),
		Length:        int64(len(payload)),
		SupportsRange: false, // No range support.
	})

	// Simulate failure - should not be able to resume.
	ft.SetFailAfter(url, 2500)

	client := &http.Client{
		Transport: New(ft, WithMaxRetries(3)),
	}

	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	defer resp.Body.Close()

	got, err := io.ReadAll(resp.Body)
	if err == nil {
		t.Error("expected read error due to no range support and failure")
	}

	// Should only get partial data.
	if len(got) >= len(payload) {
		t.Errorf("got %d bytes, expected less than %d", len(got), len(payload))
	}
}

// TestIfRange_ETag_Matches_AllowsResume tests If-Range with ETag validation.
func TestIfRange_ETag_Matches_AllowsResume(t *testing.T) {
	url := "https://example.com/if-range-etag"
	payload := testutil.GenerateTestData(7500)
	etag := `"strong-etag"`

	ft := testutil.NewFakeTransport()
	ft.Add(url, &testutil.FakeResource{
		Data:          bytes.NewReader(payload),
		Length:        int64(len(payload)),
		SupportsRange: true,
		ETag:          etag,
	})

	// Simulate failure to trigger resume.
	failCount := 0
	var muFail sync.Mutex
	ft.ResponseHook = func(resp *http.Response) {
		muFail.Lock()
		fc := failCount
		if resp.Request.Method == http.MethodGet && fc == 0 {
			failCount = fc + 1
			muFail.Unlock()
			// First request fails after 3000 bytes.
			resp.Body = testutil.NewFlakyReader(
				bytes.NewReader(payload),
				int64(len(payload)),
				3000,
			)
			return
		}
		muFail.Unlock()
	}

	client := &http.Client{
		Transport: New(ft, WithMaxRetries(3)),
	}

	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	defer resp.Body.Close()

	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}

	testutil.AssertDataEquals(t, got, payload)

	// Check If-Range header on resume request.
	headers := ft.GetRequestHeaders(url)
	foundIfRange := false
	for _, h := range headers {
		if h.Get("Range") != "" {
			if ifRange := h.Get("If-Range"); ifRange == etag {
				foundIfRange = true
				break
			}
		}
	}

	if !foundIfRange {
		t.Error("expected If-Range header with ETag on resume")
	}
}

// TestIfRange_ETag_ChangedOnResume_RejectsResume tests ETag change detection.
func TestIfRange_ETag_ChangedOnResume_RejectsResume(t *testing.T) {
	url := "https://example.com/etag-changed"
	payload := testutil.GenerateTestData(5000)
	originalETag := `"original"`
	changedETag := `"changed"`

	ft := testutil.NewFakeTransport()
	ft.Add(url, &testutil.FakeResource{
		Data:          bytes.NewReader(payload),
		Length:        int64(len(payload)),
		SupportsRange: true,
		ETag:          originalETag,
	})

	// Change ETag on resume attempt.
	ft.ResponseHook = func(resp *http.Response) {
		if resp.Request.Header.Get("Range") != "" {
			// Simulate resource change.
			resp.StatusCode = http.StatusOK
			resp.Status = "200 OK"
			resp.Header.Set("ETag", changedETag)
			resp.Header.Del("Content-Range")
			resp.Body = io.NopCloser(testutil.NewFlakyReader(
				bytes.NewReader(payload),
				int64(len(payload)),
				0,
			))
		}
	}

	ft.SetFailAfter(url, 2500)

	client := &http.Client{
		Transport: New(ft, WithMaxRetries(3)),
	}

	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	_, err = io.ReadAll(resp.Body)
	if err == nil ||
		err.Error() != "resumable: server returned 200 to a range request; resource may have changed" {
		t.Errorf("expected resource change error, got: %v", err)
	}
}

// TestIfRange_LastModified_Matches_AllowsResume tests If-Range with Last-Modified
func TestIfRange_LastModified_Matches_AllowsResume(t *testing.T) {
	url := "https://example.com/if-range-lm"
	payload := testutil.GenerateTestData(6000)
	lastModified := "Wed, 21 Oct 2015 07:28:00 GMT"

	ft := testutil.NewFakeTransport()
	ft.Add(url, &testutil.FakeResource{
		Data:          bytes.NewReader(payload),
		Length:        int64(len(payload)),
		SupportsRange: true,
		LastModified:  lastModified,
		// No ETag, so should use Last-Modified
	})

	// Simulate failure
	ft.SetFailAfter(url, 3000)

	client := &http.Client{
		Transport: New(ft, WithMaxRetries(3)),
	}

	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	defer resp.Body.Close()

	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}

	testutil.AssertDataEquals(t, got, payload)

	// Check If-Range uses Last-Modified
	headers := ft.GetRequestHeaders(url)
	foundIfRange := false
	for _, h := range headers {
		if h.Get("Range") != "" {
			if ifRange := h.Get("If-Range"); ifRange == lastModified {
				foundIfRange = true
				break
			}
		}
	}

	if !foundIfRange {
		t.Error("expected If-Range header with Last-Modified on resume")
	}
}

// TestIfRange_LastModified_ChangedOnResume_RejectsResume tests Last-Modified change detection
func TestIfRange_LastModified_ChangedOnResume_RejectsResume(t *testing.T) {
	url := "https://example.com/lm-changed"
	payload := testutil.GenerateTestData(5000)
	originalLM := "Wed, 21 Oct 2015 07:28:00 GMT"
	changedLM := "Thu, 22 Oct 2015 08:30:00 GMT"

	ft := testutil.NewFakeTransport()
	ft.Add(url, &testutil.FakeResource{
		Data:          bytes.NewReader(payload),
		Length:        int64(len(payload)),
		SupportsRange: true,
		LastModified:  originalLM,
	})

	// Change Last-Modified on resume
	ft.ResponseHook = func(resp *http.Response) {
		if resp.Request.Header.Get("Range") != "" {
			// Simulate resource change
			resp.StatusCode = http.StatusOK
			resp.Status = "200 OK"
			resp.Header.Set("Last-Modified", changedLM)
			resp.Header.Del("Content-Range")
			resp.Body = io.NopCloser(testutil.NewFlakyReader(
				bytes.NewReader(payload),
				int64(len(payload)),
				0,
			))
		}
	}

	ft.SetFailAfter(url, 2500)

	client := &http.Client{
		Transport: New(ft, WithMaxRetries(3)),
	}

	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	_, err = io.ReadAll(resp.Body)
	if err == nil ||
		err.Error() != "resumable: server returned 200 to a range request; resource may have changed" {
		t.Errorf("expected resource change error, got: %v", err)
	}
}

// TestIfRange_RequiredButUnavailable_MissingRejected tests when no validator is available
func TestIfRange_RequiredButUnavailable_MissingRejected(t *testing.T) {
	url := "https://example.com/no-validator"
	payload := testutil.GenerateTestData(5000)

	ft := testutil.NewFakeTransport()
	ft.Add(url, &testutil.FakeResource{
		Data:          bytes.NewReader(payload),
		Length:        int64(len(payload)),
		SupportsRange: true,
		// No ETag or LastModified
	})

	ft.SetFailAfter(url, 2500)

	client := &http.Client{
		Transport: New(ft, WithMaxRetries(3)),
	}

	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	_, err = io.ReadAll(resp.Body)
	// Safer behavior: do not attempt resume without a validator. Expect an
	// error to be surfaced when the initial stream fails and cannot resume.
	if err == nil {
		t.Error("expected error due to missing resume validator")
	}
}

// TestIfRange_WeakETag_Present_UsesLastModified_AllowsResume tests weak ETags fall back to Last-Modified
func TestIfRange_WeakETag_Present_UsesLastModified_AllowsResume(t *testing.T) {
	url := "https://example.com/weak-etag"
	payload := testutil.GenerateTestData(10000)
	lastModified := "Mon, 02 Jan 2006 15:04:05 MST"

	ft := testutil.NewFakeTransport()
	ft.Add(url, &testutil.FakeResource{
		Data:          bytes.NewReader(payload),
		Length:        int64(len(payload)),
		SupportsRange: true,
		ETag:          `W/"weak-etag"`, // Weak ETag
		LastModified:  lastModified,
	})

	// Simulate failure
	ft.SetFailAfter(url, 5000)

	client := &http.Client{
		Transport: New(ft, WithMaxRetries(3)),
	}

	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	defer resp.Body.Close()

	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}

	testutil.AssertDataEquals(t, got, payload)

	// Should use Last-Modified for If-Range, not weak ETag
	headers := ft.GetRequestHeaders(url)
	for _, h := range headers {
		if h.Get("Range") != "" {
			ifRange := h.Get("If-Range")
			if ifRange == `W/"weak-etag"` {
				t.Error("should not use weak ETag for If-Range")
			}
			if ifRange != lastModified {
				t.Errorf("expected If-Range with Last-Modified, got %q", ifRange)
			}
		}
	}
}

// TestGzipContentEncoding_DisablesResume tests that Content-Encoding disables resume
func TestGzipContentEncoding_DisablesResume(t *testing.T) {
	url := "https://example.com/gzip"
	payload := testutil.GenerateTestData(12000)

	ft := testutil.NewFakeTransport()
	ft.Add(url, &testutil.FakeResource{
		Data:          bytes.NewReader(payload),
		Length:        int64(len(payload)),
		SupportsRange: true,
		Headers: http.Header{
			"Content-Encoding": []string{"gzip"},
		},
	})

	// Simulate failure
	ft.SetFailAfter(url, 6000)

	client := &http.Client{
		Transport: New(ft, WithMaxRetries(3)),
	}

	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	defer resp.Body.Close()

	got, err := io.ReadAll(resp.Body)
	// Should fail because Content-Encoding prevents resume
	if err == nil {
		t.Error("expected error due to Content-Encoding preventing resume")
	}

	// Should only have partial data
	if len(got) >= len(payload) {
		t.Errorf("got %d bytes, expected less due to failure", len(got))
	}
}

// TestResumeHeaders_ScrubbedAndIdentityEncoding tests header handling on resume
func TestResumeHeaders_ScrubbedAndIdentityEncoding(t *testing.T) {
	url := "https://example.com/headers"
	payload := testutil.GenerateTestData(5000)

	ft := testutil.NewFakeTransport()
	ft.Add(url, &testutil.FakeResource{
		Data:          bytes.NewReader(payload),
		Length:        int64(len(payload)),
		SupportsRange: true,
		ETag:          `"test"`,
	})

	// Check headers on resume
	ft.RequestHook = func(req *http.Request) {
		if req.Header.Get("Range") != "" {
			// Check that Accept-Encoding is set to identity
			if ae := req.Header.Get("Accept-Encoding"); ae != "identity" {
				t.Errorf("expected Accept-Encoding: identity, got: %q", ae)
			}
			// Check that conditional headers are removed
			if req.Header.Get("If-Modified-Since") != "" {
				t.Error("If-Modified-Since should be removed on resume")
			}
			if req.Header.Get("If-None-Match") != "" {
				t.Error("If-None-Match should be removed on resume")
			}
		}
	}

	ft.SetFailAfter(url, 2500)

	client := &http.Client{
		Transport: New(ft, WithMaxRetries(3)),
	}

	// Create request with various headers
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Accept-Encoding", "gzip, deflate")
	req.Header.Set("If-Modified-Since", "Wed, 21 Oct 2015 07:28:00 GMT")
	req.Header.Set("If-None-Match", `"other"`)

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	testutil.AssertDataEquals(t, got, payload)
}

// TestRangeRequest_Initial tests resume with initial Range request
func TestRangeRequest_Initial(t *testing.T) {
	url := "https://example.com/range-initial"
	payload := testutil.GenerateTestData(10240)

	ft := testutil.NewFakeTransport()
	ft.Add(url, &testutil.FakeResource{
		Data:          bytes.NewReader(payload),
		Length:        int64(len(payload)),
		SupportsRange: true,
		ETag:          `"range-test"`,
	})

	// Simulate failure on range request
	failCount := 0
	var muRange sync.Mutex
	ft.ResponseHook = func(resp *http.Response) {
		muRange.Lock()
		fc := failCount
		if resp.Request.Header.Get("Range") == "bytes=1024-5119" && fc == 0 {
			failCount = fc + 1
			muRange.Unlock()
			// Fail after 2000 bytes of the range
			rangeData := payload[1024:5120]
			resp.Body = testutil.NewFlakyReader(
				bytes.NewReader(rangeData),
				int64(len(rangeData)),
				2000,
			)
			return
		}
		muRange.Unlock()
	}

	// Create request with initial Range header
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Range", "bytes=1024-5119")

	client := &http.Client{
		Transport: New(ft, WithMaxRetries(3)),
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	defer resp.Body.Close()

	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}

	want := payload[1024:5120]
	testutil.AssertDataEquals(t, got, want)

	// Check resume happened with adjusted range
	headers := ft.GetRequestHeaders(url)
	foundResume := false
	for _, h := range headers {
		rangeHeader := h.Get("Range")
		if rangeHeader != "" && rangeHeader != "bytes=1024-5119" {
			foundResume = true
			t.Logf("Resume range: %s", rangeHeader)
		}
	}

	if !foundResume {
		t.Error("expected resume with adjusted range")
	}
}

// Additional range request tests for comprehensive coverage
func TestRangeInitial_ZeroToN_NoCuts_Succeeds(t *testing.T) {
	url := "https://example.com/range-0-n"
	payload := testutil.GenerateTestData(5000)

	ft := testutil.NewFakeTransport()
	ft.Add(url, &testutil.FakeResource{
		Data:          bytes.NewReader(payload),
		Length:        int64(len(payload)),
		SupportsRange: true,
	})

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Range", "bytes=0-2499")

	client := &http.Client{Transport: New(ft)}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}

	want := payload[0:2500]
	testutil.AssertDataEquals(t, got, want)
}

func TestRangeInitial_MidSpan_NoCuts_Succeeds(t *testing.T) {
	url := "https://example.com/range-mid"
	payload := testutil.GenerateTestData(5000)

	ft := testutil.NewFakeTransport()
	ft.Add(url, &testutil.FakeResource{
		Data:          bytes.NewReader(payload),
		Length:        int64(len(payload)),
		SupportsRange: true,
	})

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Range", "bytes=1000-1999")

	client := &http.Client{Transport: New(ft)}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}

	want := payload[1000:2000]
	testutil.AssertDataEquals(t, got, want)
}

func TestRangeInitial_FromNToEnd_NoCuts_Succeeds(t *testing.T) {
	url := "https://example.com/range-to-end"
	payload := testutil.GenerateTestData(5000)

	ft := testutil.NewFakeTransport()
	ft.Add(url, &testutil.FakeResource{
		Data:          bytes.NewReader(payload),
		Length:        int64(len(payload)),
		SupportsRange: true,
	})

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Range", "bytes=3000-")

	client := &http.Client{Transport: New(ft)}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}

	want := payload[3000:]
	testutil.AssertDataEquals(t, got, want)
}

func TestRangeInitial_ZeroToN_WithCut_Resumes(t *testing.T) {
	url := "https://example.com/range-0-n-cut"
	payload := testutil.GenerateTestData(5000)

	ft := testutil.NewFakeTransport()
	ft.Add(url, &testutil.FakeResource{
		Data:          bytes.NewReader(payload),
		Length:        int64(len(payload)),
		SupportsRange: true,
		ETag:          `"test"`,
	})

	// Fail the range request partway through
	failCount := 0
	ft.ResponseHook = func(resp *http.Response) {
		if resp.Request.Header.Get("Range") == "bytes=0-2499" && failCount == 0 {
			failCount++
			resp.Body = testutil.NewFlakyReader(
				bytes.NewReader(payload[0:2500]),
				int64(len(payload[0:2500])),
				1000,
			)
		}
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Range", "bytes=0-2499")

	client := &http.Client{Transport: New(ft, WithMaxRetries(3))}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}

	want := payload[0:2500]
	testutil.AssertDataEquals(t, got, want)

	// Verify resume happened
	headers := ft.GetRequestHeaders(url)
	foundResume := false
	for _, h := range headers {
		rangeHeader := h.Get("Range")
		if rangeHeader != "" && rangeHeader != "bytes=0-2499" {
			foundResume = true
			if rangeHeader != "bytes=1000-2499" {
				t.Errorf("expected resume at bytes=1000-2499, got: %s", rangeHeader)
			}
		}
	}

	if !foundResume {
		t.Error("expected resume")
	}
}

func TestRangeInitial_MidSpan_WithMultipleCuts_Resumes(t *testing.T) {
	url := "https://example.com/range-mid-cuts"
	payload := testutil.GenerateTestData(10000)

	ft := testutil.NewFakeTransport()
	ft.Add(url, &testutil.FakeResource{
		Data:          bytes.NewReader(payload),
		Length:        int64(len(payload)),
		SupportsRange: true,
		ETag:          `"test"`,
	})

	// Multiple failures on the range request
	failCount := 0
	var muCut sync.Mutex
	ft.ResponseHook = func(resp *http.Response) {
		rangeHeader := resp.Request.Header.Get("Range")
		muCut.Lock()
		fc := failCount
		if rangeHeader == "bytes=2000-5999" && fc == 0 {
			failCount = fc + 1
			muCut.Unlock()
			resp.Body = testutil.NewFlakyReader(
				bytes.NewReader(payload[2000:6000]),
				int64(len(payload[2000:6000])),
				1000,
			)
			return
		} else if rangeHeader == "bytes=3000-5999" && fc == 1 {
			failCount = fc + 1
			muCut.Unlock()
			resp.Body = testutil.NewFlakyReader(
				bytes.NewReader(payload[3000:6000]),
				int64(len(payload[3000:6000])),
				1500,
			)
			return
		}
		muCut.Unlock()
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Range", "bytes=2000-5999")

	client := &http.Client{Transport: New(ft, WithMaxRetries(5))}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}

	want := payload[2000:6000]
	testutil.AssertDataEquals(t, got, want)

	// Check that multiple resumes happened.
	reqs := ft.GetRequests()
	var rangeCount int
	for _, r := range reqs {
		if r.Header.Get("Range") != "" {
			rangeCount++
		}
	}

	if rangeCount < 3 {
		t.Errorf("expected at least 3 range requests, got %d", rangeCount)
	}
}

func TestRangeInitial_FromNToEnd_WithCut_Resumes(t *testing.T) {
	url := "https://example.com/range-to-end-cut"
	payload := testutil.GenerateTestData(10000)

	ft := testutil.NewFakeTransport()
	ft.Add(url, &testutil.FakeResource{
		Data:          bytes.NewReader(payload),
		Length:        int64(len(payload)),
		SupportsRange: true,
		ETag:          `"test"`,
	})

	// Fail the open-ended range request
	failCount := 0
	var muOpen sync.Mutex
	ft.ResponseHook = func(resp *http.Response) {
		muOpen.Lock()
		fc := failCount
		if resp.Request.Header.Get("Range") == "bytes=7000-" && fc == 0 {
			failCount = fc + 1
			muOpen.Unlock()
			resp.Body = testutil.NewFlakyReader(
				bytes.NewReader(payload[7000:]),
				int64(len(payload[7000:])),
				1500,
			)
			return
		}
		muOpen.Unlock()
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Range", "bytes=7000-")

	client := &http.Client{Transport: New(ft, WithMaxRetries(3))}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}

	want := payload[7000:]
	testutil.AssertDataEquals(t, got, want)

	// Verify resume happened
	headers := ft.GetRequestHeaders(url)
	foundResume := false
	for _, h := range headers {
		rangeHeader := h.Get("Range")
		if rangeHeader != "" && rangeHeader != "bytes=7000-" {
			foundResume = true
			// Accept either open-ended or closed range
			if rangeHeader != "bytes=8500-" && rangeHeader != "bytes=8500-9999" {
				t.Errorf("expected resume at bytes=8500- or bytes=8500-9999, got: %s", rangeHeader)
			}
		}
	}

	if !foundResume {
		t.Error("expected resume")
	}
}

func TestRangeInitial_ResumeHeaderStart_Correct(t *testing.T) {
	url := "https://example.com/range-header-check"
	payload := testutil.GenerateTestData(5000)

	ft := testutil.NewFakeTransport()
	ft.Add(url, &testutil.FakeResource{
		Data:          bytes.NewReader(payload),
		Length:        int64(len(payload)),
		SupportsRange: true,
		ETag:          `"test"`,
	})

	// Fail at exactly 1234 bytes
	failCount := 0
	var muHdr sync.Mutex
	ft.ResponseHook = func(resp *http.Response) {
		muHdr.Lock()
		fc := failCount
		if resp.Request.Header.Get("Range") == "bytes=1000-2999" && fc == 0 {
			failCount = fc + 1
			muHdr.Unlock()
			rangeData := payload[1000:3000]
			resp.Body = testutil.NewFlakyReader(
				bytes.NewReader(rangeData),
				int64(len(rangeData)),
				234,
			) // Will have read 1234 total
			return
		}
		muHdr.Unlock()
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Range", "bytes=1000-2999")

	client := &http.Client{Transport: New(ft, WithMaxRetries(3))}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}

	want := payload[1000:3000]
	testutil.AssertDataEquals(t, got, want)

	// Check the resume request has correct start position
	headers := ft.GetRequestHeaders(url)
	for _, h := range headers {
		rangeHeader := h.Get("Range")
		if rangeHeader != "" && rangeHeader != "bytes=1000-2999" {
			if rangeHeader != "bytes=1234-2999" {
				t.Errorf("expected resume at bytes=1234-2999, got: %s", rangeHeader)
			}
			break
		}
	}
}
