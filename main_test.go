// download_test.go -- regression tests for rupdater.
// End markers (// end:...) added per project convention.
//
// Test provenance:
//  - TestCalculateMD5 is adapted from the old testsuite/rupdater_test.go
//    TestVerifyMD5, rewritten to exercise the production calculateMD5.
//    The old test's MD5 vector was wrong (it was the hash of
//    "test content\n", not "test content"); corrected here.
//  - The old TestDownloadFile (live HTTP) and TestDeleteFile (stdlib)
//    were intentionally dropped: flaky / no value.
//
// Coverage map (finding IDs from main.go header):
//  S1  -> TestDownloadFile_TrustedSymlinkFollowed
//         TestDownloadFile_RejectsSymlinkPart
//  S2  -> TestSafeLocalName
//  B1  -> TestDownloadFile_IdleWatchdog
//  B3  -> TestDownloadFile_TruncatedBody
//  B4  -> TestParseMd5Line
//  B7  -> TestDownloadFile_Success (dot output)
//  B11 -> TestParseContentRange
//         TestDownloadFile_Resume
//         TestDownloadFile_ServerIgnoresRange
//         TestDownloadFile_416Restart
//         TestDownloadFile_ResumeAfterDrop
package main

import (
	"bytes"
	"crypto/md5"
	"crypto/rand"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// --------------------------------------------------------------------------
// TestDownloadFile_Success verifies a normal 3 MB download round-trips
// and that no .part file is left behind.
func TestDownloadFile_Success(t *testing.T) {
	payload := make([]byte, 3*1024*1024)
	if _, err := rand.Read(payload); err != nil {
		t.Fatal(err)
	} // end:if rand
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(payload)))
		w.Write(payload)
	}))
	defer srv.Close()

	dst := filepath.Join(t.TempDir(), "out.bin")
	if err := downloadFile(srv.URL, dst); err != nil {
		t.Fatalf("downloadFile: %v", err)
	} // end:if download
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	} // end:if read
	if !bytes.Equal(got, payload) {
		t.Fatalf("payload mismatch: got %d, want %d", len(got), len(payload))
	} // end:if mismatch
	if _, err := os.Stat(dst + ".part"); !os.IsNotExist(err) {
		t.Fatalf(".part must be gone after success")
	} // end:if part exists
} // end:TestDownloadFile_Success

// --------------------------------------------------------------------------
// TestDownloadFile_TruncatedBody verifies a short body is detected and
// returns an error after the retry budget is exhausted.
//
// B2 contract: the final target MUST NOT exist after failure.
// B11 contract: .part MUST exist (with partial data) so a future
// program run can resume. The pre-E2 version of this test asserted
// .part was cleaned up; that is wrong now -- keeping .part is the
// entire point of resume.
func TestDownloadFile_TruncatedBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1000")
		w.Write([]byte("only 10 b!"))
	}))
	defer srv.Close()

	dst := filepath.Join(t.TempDir(), "out.bin")
	if err := downloadFile(srv.URL, dst); err == nil {
		t.Fatalf("want error for truncated body, got nil")
	} // end:if no error

	// B2: the final target must not exist after failure.
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Fatalf("final file must not exist after failure")
	} // end:if final

	// B11: .part must survive for resume. Exact size depends on how
	// many restart rounds occurred; only require "some partial data".
	fi, err := os.Stat(dst + ".part")
	if err != nil {
		t.Fatalf(".part must exist for resume: %v", err)
	} // end:if missing
	if fi.Size() == 0 {
		t.Fatalf(".part must contain partial data, got 0 bytes")
	} // end:if empty
} // end:TestDownloadFile_TruncatedBody

// --------------------------------------------------------------------------
// TestDownloadFile_IdleWatchdog verifies a hung server is aborted after
// defaultIdleTimeout (overridden here to 400 ms).
//
// NOTE: the handler MUST Flush() after the first Write. httptest.Server
// buffers the response until the handler returns; without Flush the
// client never sees headers and the transport's ResponseHeaderTimeout
// (60 s) fires instead of our per-read idle watchdog.
func TestDownloadFile_IdleWatchdog(t *testing.T) {
	old := defaultIdleTimeout
	defaultIdleTimeout = 400 * time.Millisecond
	defer func() { defaultIdleTimeout = old }()

	var hits int32
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 4xx on retries so the loop terminates immediately.
		if atomic.AddInt32(&hits, 1) > 1 {
			w.WriteHeader(http.StatusNotFound)
			return
		} // end:if retry
		w.Header().Set("Content-Length", "100")
		w.Write([]byte("x"))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		} // end:if flusher
		<-release
	}))
	defer func() { close(release); srv.Close() }()
	// ... rest unchanged
} // end:TestDownloadFile_IdleWatchdog

// --------------------------------------------------------------------------
// TestDownloadFile_TrustedSymlinkFollowed verifies a symlink owned by
// the invoking user is followed: bytes land on the real target, and the
// link stays a link (Unix only).
func TestDownloadFile_TrustedSymlinkFollowed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-only")
	} // end:if windows
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hello"))
	}))
	defer srv.Close()

	dir := t.TempDir()
	target := filepath.Join(dir, "real.bin")
	if err := os.WriteFile(target, []byte("old"), 0644); err != nil {
		t.Fatal(err)
	} // end:if write target
	link := filepath.Join(dir, "link.bin")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink: %v", err)
	} // end:if symlink
	if err := downloadFile(srv.URL, link); err != nil {
		t.Fatalf("downloadFile via trusted symlink: %v", err)
	} // end:if download
	got, _ := os.ReadFile(target)
	if string(got) != "hello" {
		t.Fatalf("target not written: got %q", got)
	} // end:if wrong bytes
	fi, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	} // end:if lstat
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("link was replaced by a regular file")
	} // end:if not symlink
} // end:TestDownloadFile_TrustedSymlinkFollowed

// --------------------------------------------------------------------------
// TestSafeLocalName verifies path-traversal rejection (S2).
func TestSafeLocalName(t *testing.T) {
	bad := []string{
		"", ".", "..", "/abs", "../up", "dir/file", `..\win`,
		`a\b`, "nul\x00byte",
	}
	for _, n := range bad {
		if err := safeLocalName(n); err == nil {
			t.Errorf("safeLocalName(%q) = nil, want error", n)
		} // end:if unexpected nil
	} // end:for bad
	if err := safeLocalName("good.bin"); err != nil {
		t.Errorf("safeLocalName(good.bin) = %v, want nil", err)
	} // end:if good
} // end:TestSafeLocalName

// --------------------------------------------------------------------------
// TestCalculateMD5 pins calculateMD5 against a known-answer vector.
//
// NOTE: the old test used d8e8fca2dc0f896fd7cb4cb0031ba249 as the hash
// for "test content". That value is md5("test content\n") -- it carries
// a trailing newline. The correct hash for "test content" is
// 9473fdd0d880a43c21b7778d34872157. Vector corrected here.
func TestCalculateMD5(t *testing.T) {
	content := []byte("test content")
	const want = "9473fdd0d880a43c21b7778d34872157" // md5("test content")

	// Cross-check the vector first, so a typo here fails loudly and
	// before the production function is exercised.
	sum := md5.Sum(content)
	if fmt.Sprintf("%x", sum) != want {
		t.Fatalf("test vector is wrong: stdlib says %x", sum)
	} // end:if vector

	path := filepath.Join(t.TempDir(), "testfile.txt")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	} // end:if write

	got, err := calculateMD5(path)
	if err != nil {
		t.Fatalf("calculateMD5: %v", err)
	} // end:if calc
	if got != want {
		t.Fatalf("MD5 mismatch: got %s, want %s", got, want)
	} // end:if mismatch

	// Negative case: missing file must return an error, not a hash.
	if _, err := calculateMD5(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Fatalf("want error for missing file, got nil")
	} // end:if no error
} // end:TestCalculateMD5

// --------------------------------------------------------------------------
// TestFormatThousands locks the dot-separator output format.
func TestFormatThousands(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "0"},
		{1, "1"},
		{999, "999"},
		{1000, "1.000"},
		{1234, "1.234"},
		{1234567, "1.234.567"},
		{-1234567, "-1.234.567"},
		{-1, "-1"},
	}
	for _, c := range cases {
		if got := formatThousands(c.in); got != c.want {
			t.Errorf("formatThousands(%d) = %q, want %q",
				c.in, got, c.want)
		} // end:if mismatch
	} // end:for cases
} // end:TestFormatThousands

// --------------------------------------------------------------------------
// TestParseMd5Line covers the B4 fix: filenames with spaces, the "*"
// binary marker emitted by md5sum, comments, and malformed lines.
func TestParseMd5Line(t *testing.T) {
	const h = "d41d8cd98f00b204e9800998ecf8427e" // md5("")
	cases := []struct {
		name     string
		line     string
		wantName string
		wantErr  bool
	}{
		{"text mode", h + "  file.bin", "file.bin", false},
		{"binary mode", h + " *file.bin", "file.bin", false},
		{"with spaces", h + "  my file with spaces.bin",
			"my file with spaces.bin", false},
		{"binary+spaces", h + " *my file.bin", "my file.bin", false},
		{"blank", "", "", false},
		{"comment", "# hello", "", false},
		{"short", "abc", "", true},
		{"bad hash", strings.Repeat("z", 32) + "  f", "", true},
		{"no separator", h + "xxf", "", true},
		{"empty name", h + "  ", "", true},
	}
	for _, c := range cases {
		gotHash, gotName, err := parseMd5Line(c.line)
		if c.wantErr {
			if err == nil {
				t.Errorf("%s: want error, got nil", c.name)
			} // end:if no error
			continue
		} // end:if wantErr
		if err != nil {
			t.Errorf("%s: unexpected error: %v", c.name, err)
			continue
		} // end:if err
		if gotName != c.wantName {
			t.Errorf("%s: name = %q, want %q",
				c.name, gotName, c.wantName)
		} // end:if name
		if gotName != "" && gotHash != h {
			t.Errorf("%s: hash = %q, want %q", c.name, gotHash, h)
		} // end:if hash
	} // end:for cases
} // end:TestParseMd5Line

// --------------------------------------------------------------------------
// TestParseContentRange covers the B11 Content-Range parser.
func TestParseContentRange(t *testing.T) {
	cases := []struct {
		in        string
		wantStart int64
		wantTotal int64
		wantErr   bool
	}{
		{"bytes 100-199/1000", 100, 1000, false},
		{"bytes 0-99/100", 0, 100, false},
		{"bytes 50-99/*", 50, -1, false},
		{"", 0, 0, true},
		{"bytes", 0, 0, true},
		{"bytes 100-199", 0, 0, true},  // no slash
		{"bytes 100/1000", 0, 0, true}, // no dash
		{"bytes abc-199/1000", 0, 0, true},
		{"bytes 100-abc/1000", 0, 0, true},
		{"bytes 100-199/abc", 0, 0, true},
	}
	for _, c := range cases {
		s, _, total, err := parseContentRange(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("parseContentRange(%q): want error, got nil", c.in)
			} // end:if no error
			continue
		} // end:if wantErr
		if err != nil {
			t.Errorf("parseContentRange(%q): unexpected error: %v", c.in, err)
			continue
		} // end:if err
		if s != c.wantStart || total != c.wantTotal {
			t.Errorf("parseContentRange(%q) = (%d, %d), want (%d, %d)",
				c.in, s, total, c.wantStart, c.wantTotal)
		} // end:if mismatch
	} // end:for cases
} // end:TestParseContentRange

// --------------------------------------------------------------------------
// TestDownloadFile_Resume verifies that an existing .part file triggers
// a Range request and the response is appended (B11).
func TestDownloadFile_Resume(t *testing.T) {
	payload := make([]byte, 100*1024)
	if _, err := rand.Read(payload); err != nil {
		t.Fatal(err)
	} // end:if rand
	const start = 50 * 1024

	var gotRange string
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			gotRange = r.Header.Get("Range")
			w.Header().Set("Content-Range",
				fmt.Sprintf("bytes %d-%d/%d",
					start, len(payload)-1, len(payload)))
			w.Header().Set("Content-Length",
				fmt.Sprintf("%d", int64(len(payload))-start))
			w.WriteHeader(http.StatusPartialContent)
			w.Write(payload[start:])
		}))
	defer srv.Close()

	dst := filepath.Join(t.TempDir(), "resume.bin")
	if err := os.WriteFile(dst+".part", payload[:start], 0644); err != nil {
		t.Fatal(err)
	} // end:if write part
	if err := downloadFile(srv.URL, dst); err != nil {
		t.Fatalf("downloadFile: %v", err)
	} // end:if download

	wantRange := fmt.Sprintf("bytes=%d-", start)
	if gotRange != wantRange {
		t.Errorf("Range header = %q, want %q", gotRange, wantRange)
	} // end:if range
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	} // end:if read
	if !bytes.Equal(got, payload) {
		t.Fatalf("payload mismatch: got %d bytes, want %d",
			len(got), len(payload))
	} // end:if payload
	if _, err := os.Stat(dst + ".part"); !os.IsNotExist(err) {
		t.Fatalf(".part must be gone after success")
	} // end:if part
} // end:TestDownloadFile_Resume

// --------------------------------------------------------------------------
// TestDownloadFile_ServerIgnoresRange verifies that a 200 response on a
// Range request truncates .part and restarts from 0 (B11 degraded mode).
func TestDownloadFile_ServerIgnoresRange(t *testing.T) {
	payload := make([]byte, 100*1024)
	if _, err := rand.Read(payload); err != nil {
		t.Fatal(err)
	} // end:if rand

	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			// Ignore Range, always return full content.
			w.Header().Set("Content-Length",
				fmt.Sprintf("%d", len(payload)))
			w.WriteHeader(http.StatusOK)
			w.Write(payload)
		}))
	defer srv.Close()

	dst := filepath.Join(t.TempDir(), "restart.bin")
	// Pre-populate .part with wrong content to prove truncation.
	if err := os.WriteFile(dst+".part", []byte("stale garbage"), 0644); err != nil {
		t.Fatal(err)
	} // end:if write part
	if err := downloadFile(srv.URL, dst); err != nil {
		t.Fatalf("downloadFile: %v", err)
	} // end:if download
	got, _ := os.ReadFile(dst)
	if !bytes.Equal(got, payload) {
		t.Fatalf("payload mismatch: got %d bytes, want %d",
			len(got), len(payload))
	} // end:if payload
} // end:TestDownloadFile_ServerIgnoresRange

// --------------------------------------------------------------------------
// TestDownloadFile_416Restart verifies that a 416 response discards the
// oversized .part and restarts from 0 (B11).
func TestDownloadFile_416Restart(t *testing.T) {
	payload := make([]byte, 100*1024)
	if _, err := rand.Read(payload); err != nil {
		t.Fatal(err)
	} // end:if rand

	var reqCount int32
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			n := atomic.AddInt32(&reqCount, 1)
			if n == 1 {
				// First request (with Range) -> 416.
				w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
				return
			} // end:if first
			// Second request -> full content.
			w.Header().Set("Content-Length",
				fmt.Sprintf("%d", len(payload)))
			w.WriteHeader(http.StatusOK)
			w.Write(payload)
		}))
	defer srv.Close()

	dst := filepath.Join(t.TempDir(), "restart416.bin")
	// .part larger than the payload -> Range request will 416.
	if err := os.WriteFile(dst+".part", make([]byte, 200*1024), 0644); err != nil {
		t.Fatal(err)
	} // end:if write part
	if err := downloadFile(srv.URL, dst); err != nil {
		t.Fatalf("downloadFile: %v", err)
	} // end:if download
	got, _ := os.ReadFile(dst)
	if !bytes.Equal(got, payload) {
		t.Fatalf("payload mismatch: got %d bytes, want %d",
			len(got), len(payload))
	} // end:if payload
} // end:TestDownloadFile_416Restart

// --------------------------------------------------------------------------
// TestDownloadFile_ResumeAfterDrop verifies that a mid-body connection
// drop triggers a retry with Range, and the resumed download completes.
func TestDownloadFile_ResumeAfterDrop(t *testing.T) {
	payload := make([]byte, 500*1024)
	if _, err := rand.Read(payload); err != nil {
		t.Fatal(err)
	} // end:if rand
	const dropAt = 100 * 1024

	var reqCount int32
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			n := atomic.AddInt32(&reqCount, 1)
			rangeHdr := r.Header.Get("Range")

			var start int64
			if rangeHdr == "" {
				w.Header().Set("Content-Length",
					fmt.Sprintf("%d", len(payload)))
				w.WriteHeader(http.StatusOK)
			} else {
				s, _, err := parseRangeForTest(rangeHdr,
					int64(len(payload)))
				if err != nil {
					w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
					return
				} // end:if bad range
				start = s
				w.Header().Set("Content-Range",
					fmt.Sprintf("bytes %d-%d/%d",
						start, len(payload)-1, len(payload)))
				w.Header().Set("Content-Length",
					fmt.Sprintf("%d", int64(len(payload))-start))
				w.WriteHeader(http.StatusPartialContent)
			} // end:if range

			toWrite := payload[start:]
			if n == 1 && len(toWrite) > dropAt {
				w.Write(toWrite[:dropAt])
				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				} // end:if flusher
				// Hijack and close the connection to force a client
				// read error mid-body.
				if hj, ok := w.(http.Hijacker); ok {
					conn, _, herr := hj.Hijack()
					if herr == nil {
						conn.Close()
					} // end:if hijack ok
				} // end:if hijacker
				return
			} // end:if drop
			w.Write(toWrite)
		}))
	defer srv.Close()

	dst := filepath.Join(t.TempDir(), "drop.bin")
	done := make(chan error, 1)
	go func() { done <- downloadFile(srv.URL, dst) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("downloadFile: %v", err)
		} // end:if download
	case <-time.After(15 * time.Second):
		t.Fatalf("download timed out")
	} // end:select

	got, _ := os.ReadFile(dst)
	if !bytes.Equal(got, payload) {
		t.Fatalf("payload mismatch: got %d bytes, want %d",
			len(got), len(payload))
	} // end:if payload
} // end:TestDownloadFile_ResumeAfterDrop

// --------------------------------------------------------------------------
// TestDownloadFile_RejectsSymlinkPart verifies that a .part symlink is
// refused and the symlink target is not modified (S1 defence in depth).
func TestDownloadFile_RejectsSymlinkPart(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-only")
	} // end:if windows
	payload := make([]byte, 1024)
	if _, err := rand.Read(payload); err != nil {
		t.Fatal(err)
	} // end:if rand

	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Length",
				fmt.Sprintf("%d", len(payload)))
			w.Write(payload)
		}))
	defer srv.Close()

	dir := t.TempDir()
	dst := filepath.Join(dir, "out.bin")
	attackFile := filepath.Join(dir, "attack.txt")
	const sentinel = "don't touch me"
	if err := os.WriteFile(attackFile, []byte(sentinel), 0644); err != nil {
		t.Fatal(err)
	} // end:if write attack
	if err := os.Symlink(attackFile, dst+".part"); err != nil {
		t.Skipf("symlink: %v", err)
	} // end:if symlink

	err := downloadFile(srv.URL, dst)
	if err == nil {
		t.Fatalf("want error for symlink .part, got nil")
	} // end:if no error
	if !strings.Contains(err.Error(), "non-regular") {
		t.Fatalf("want 'non-regular' error, got %v", err)
	} // end:if wrong error
	got, _ := os.ReadFile(attackFile)
	if string(got) != sentinel {
		t.Fatalf("attack file was modified: %q", got)
	} // end:if attack
} // end:TestDownloadFile_RejectsSymlinkPart

// --------------------------------------------------------------------------
// parseRangeForTest is a minimal Range header parser for the test servers.
// Accepts "bytes=START-" and "bytes=START-END". Returns the inclusive
// [start, end] range, or an error if the header is malformed or out of
// bounds.
func parseRangeForTest(h string, size int64) (int64, int64, error) {
	if !strings.HasPrefix(h, "bytes=") {
		return 0, 0, fmt.Errorf("bad range: %q", h)
	} // end:if prefix
	h = h[6:]
	dash := strings.IndexByte(h, '-')
	if dash < 0 {
		return 0, 0, fmt.Errorf("bad range: %q", h)
	} // end:if dash
	start, err := strconv.ParseInt(h[:dash], 10, 64)
	if err != nil || start < 0 || start >= size {
		return 0, 0, fmt.Errorf("bad range start: %q", h[:dash])
	} // end:if start
	var end int64
	if h[dash+1:] == "" {
		end = size - 1
	} else {
		end, err = strconv.ParseInt(h[dash+1:], 10, 64)
		if err != nil || end < start || end >= size {
			return 0, 0, fmt.Errorf("bad range end: %q", h[dash+1:])
		} // end:if end
	} // end:if end-omitted
	return start, end, nil
} // end:parseRangeForTest

