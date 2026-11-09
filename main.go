/* 	(c) 2020-2026 by ROSE_SWE, Ralph Roth
https://github.com/roseswe/rupdater2
*/
//go:generate goversioninfo -icon=main.ico -manifest=manifest.xml versioninfo.json
package main

// ============================================================================
// main.go -- rupdater: automatic update client for ROSE SWE downloads.
//
// Revision: $Id: main.go,v 1.18 2026/11/09 12:23:04 ralph Exp $   (CVS keyword; bump manually if desired)
//
// ----------------------------------------------------------------------------
// REVIEW FINDINGS -- 2026-11-09
//
// SECURITY
//  S1 (HIGH) os.Create in downloadFile followed symlinks. In a world-
//           writable directory an attacker could plant fileName ->
//           /etc/passwd and cause a privileged run to overwrite it.
//           FIXED: follow a symlink only if owned by root or the
//           invoking user; otherwise refuse with "refusing to follow a
//           symlink owned by an untrusted user". Write path is
//           .part + O_EXCL + atomic rename.
//  S2 (HIGH) Path traversal via md5sums.md5. A compromised checksum
//           file could contain "../../etc/passwd" and the program
//           would download over it. FIXED: reject absolute paths,
//           any "/" or "\" separator, "." and "..", and NUL bytes.
//  S3 (MED)  Default base URL is http://, not https://. NOTE: pass
//           --url https://... for release artifacts.
//  S4 (MED)  No size cap. NOTE: caller should cap.
//  S5 (LOW)  MD5 is cryptographically broken; kept for wire-format
//           compatibility. NOTE: migrate to SHA-256 in a future
//           format revision.
//  S6 (LOW)  md5sums.md5 is fetched unauthenticated; it is the trust
//           anchor. Mitigation: HTTPS + pinned cert.
//  S7 (RESIDUAL) Resume trusts existing .part content. An attacker
//           who can write to .part controls the first N bytes. MD5
//           verification downstream catches this for listed files;
//           md5sums.md5 itself (the bootstrap) is not verified.
//           Mitigation: run in a directory only you can write to.
//           E9 (If-Range) narrows the window but does not close it.
//
// BUGS
//  B1 (HIGH) http.Client.Timeout is a wall-clock deadline on the
//           ENTIRE exchange, including the body. On a 0.25 MB/s link
//           a 110 MB file needs ~7 min nominal and TCP backoff
//           pushes wall-clock past any fixed limit, killing the
//           transfer mid-body. FIXED: remove Client.Timeout; bound
//           dial / TLS / response-header only; add a per-read idle
//           watchdog (defaultIdleTimeout).
//  B2 (HIGH) os.Create truncated the final path immediately. FIXED:
//           write to <target>.part (O_CREATE|O_EXCL), verify length,
//           rename atomically.
//  B3 (MED)  Truncated body ending in clean io.EOF treated as
//           success. FIXED: verify totalBytes == ContentLength or
//           Content-Range total.
//  B4 (MED)  strings.Fields broke file names with spaces; the "*"
//           binary marker leaked into the name. FIXED: parseMd5Line
//           splits at column 32, strips "*".
//  B5 (MED)  os.Exit in main bypassed deferred file.Close(). FIXED:
//           main -> os.Exit(run()); run() returns the exit code.
//  B6 (MED)  os.Stat error other than IsNotExist was silently
//           treated as "file exists". FIXED: skip with clear message.
//  B7 (LOW)  Progress dots used modulo, which almost never fired.
//           FIXED: threshold counter, monotonic across retries.
//  B8 (LOW)  Deferred Close error ignored. FIXED: explicit check.
//  B9 (LOW)  Body not drained on error. FIXED: bounded drain.
//  B10 (TRIV) %v of time.Duration printed nanosecond noise. FIXED:
//           rounded to ms.
//  B11 (HIGH) No resume. 15+ minutes wasted per flake on a 246 MB
//           file at 0.25 MB/s. FIXED: retry loop with HTTP Range.
//           206 appends, 200 truncates and restarts, 416 discards
//           and restarts. Max 5 attempts per call; .part survives
//           between program runs.
//  B12 (HIGH) No ETag validation on resume. If the server's content
//           changed between partial download and resume, new bytes
//           were appended to old content, producing a corrupted
//           file that failed MD5. FIXED: capture ETag on the first
//           200 response, persist to <target>.part.meta, send
//           If-Range on subsequent Range requests. Server answers
//           206 (unchanged -> resume) or 200 (changed -> restart).
//           Weak ETags (W/"...") are rejected per RFC 7233.
//
// ENHANCEMENTS (suggested, not implemented)
//  E1 Accept context.Context as first parameter to downloadFile.
//  E3 Retry with exponential backoff (currently immediate).
//  E4 Enforce a maximum download size.
//  E5 SHA-256 checksums instead of MD5 (S5).
//  E6 Progress output to stderr so stdout can be piped cleanly.
//  E7 TLS certificate pinning for the known update host.
//  E8 Parallel downloads with a worker pool.
//  E10 Configurable maxAttempts (currently a const in the function).
//  E11 Last-Modified as fallback validator when ETag is absent.
//
// PLATFORM NOTES
//  - Targets win32/64, linux 32/64, darwin (amd64/arm64).
//  - Windows has no portable UID model; symlink ownership cannot be
//    verified in stdlib, so ANY symlink at the target path is
//    refused (default-deny).
// ============================================================================

import (
	"bufio"
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// Program version - see what(1) or mywhat
const version = "rupdater: @(#)$Id: main.go,v 1.18 2026/11/09 12:23:04 ralph Exp $"

// var BuildDate string // This will be populated during the build

// defaultIdleTimeout is the maximum time we wait for the NEXT chunk of
// body bytes. Per-read idle deadline, not a total-transfer deadline.
// Variable (not constant) so tests can override it. (Fixes B1.)
var defaultIdleTimeout = 60 * time.Second

// errUntrustedSymlink is returned when a symlink at the target path is
// owned by neither root nor the invoking user. (Fixes S1.)
var errUntrustedSymlink = errors.New(
	"refusing to follow a symlink owned by an untrusted user")

// --------------------------------------------------------------------------
// downloadFile fetches url and saves it as fileName (atomically, with
// HTTP Range resume and ETag validation).
//
// Security: refuses to follow an untrusted symlink at fileName, refuses
// to use a non-regular .part file, and refuses to persist or send a
// weak ETag (RFC 7233 forbids it with If-Range).
// Fixes: B1, B2, B3, B7, B8, B9, B10, B11, B12, S1.
func downloadFile(url, fileName string) error {
	// Start the timer. New Start
	startTime := time.Now()

	// S1/B2: apply the trusted-symlink policy.
	target, err := effectiveTarget(fileName)
	if err != nil {
		return fmt.Errorf("[!] ERROR: %v", err)
	} // end:if effectiveTarget error
	partPath := target + ".part"
	metaPath := partPath + ".meta"

	// B1: do NOT set http.Client.Timeout. Bound only connect phases.
	client := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			TLSHandshakeTimeout:   30 * time.Second,
			ResponseHeaderTimeout: 60 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			IdleConnTimeout:       90 * time.Second,
		},
	} // end:client

	// B7/B11: progress dot threshold. Monotonic across retries.
	var nextDot int64 = 2 * 1024 * 1024

	// B11: bounded retry loop with HTTP Range resume.
	const maxAttempts = 5
	var lastErr error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if attempt > 1 {
			fmt.Printf("\n[!] Retry %d/%d: ", attempt, maxAttempts)
		} // end:if attempt > 1

		// B11: size of existing .part (0 if none). Non-regular .part
		// is refused (S1).
		partSize, err := existingPartSize(partPath)
		if err != nil {
			return fmt.Errorf("[!] ERROR: %v", err)
		} // end:if existingPartSize error

		// B1: per-attempt context, cancelled by the idle watchdog.
		ctx, cancel := context.WithCancel(context.Background())

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			cancel()
			return fmt.Errorf("[!] ERROR: Failed to build request: %v", err)
		} // end:if NewRequest error

		// B11/B12: if we have partial content, ask for the remainder
		// and ask the server to validate the saved ETag via If-Range.
		if partSize > 0 {
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-", partSize))
			if savedETag, _ := readETag(metaPath); savedETag != "" {
				req.Header.Set("If-Range", savedETag)
			} // end:if savedETag
		} // end:if partSize > 0

		resp, err := client.Do(req)
		if err != nil {
			cancel()
			if ctx.Err() != nil {
				lastErr = fmt.Errorf("stalled before headers")
			} else {
				lastErr = fmt.Errorf("request failed: %v", err)
			} // end:if stalled
			continue
		} // end:if client.Do error

		// B11: write mode is determined by the response status.
		var startOffset int64
		var expectTotal int64 = -1

		switch resp.StatusCode {
		case http.StatusOK:
			// B12: If-Range mismatch (file changed) or server does not
			// support Range. Truncate .part, capture new ETag, restart.
			if partSize > 0 {
				fmt.Printf("\n[!] Server responded with full content " +
					"(file changed or no Range support); restarting")
				nextDot = 2 * 1024 * 1024
			} // end:if partSize > 0
			// Persist the new ETag (if any) for the next attempt.
			if newETag := resp.Header.Get("ETag"); newETag != "" {
				_ = writeETag(metaPath, newETag)
			} else {
				os.Remove(metaPath) // no ETag to persist
			} // end:if newETag
			startOffset = 0
			expectTotal = resp.ContentLength
		case http.StatusPartialContent:
			// B12: server honored If-Range (ETag matched) and Range.
			// Verify the offset returned matches what we asked for.
			cr := resp.Header.Get("Content-Range")
			s, _, total, perr := parseContentRange(cr)
			if perr != nil {
				resp.Body.Close()
				cancel()
				lastErr = fmt.Errorf(
					"invalid Content-Range %q: %v", cr, perr)
				continue
			} // end:if parse error
			if s != partSize {
				resp.Body.Close()
				cancel()
				os.Remove(partPath)
				os.Remove(metaPath)
				lastErr = fmt.Errorf(
					"range mismatch: got %d, expected %d", s, partSize)
				continue
			} // end:if offset mismatch
			startOffset = s
			expectTotal = total
		case http.StatusRequestedRangeNotSatisfiable:
			// B11: .part is larger than the server's file, or the file
			// changed under a server that uses If-Range + 416. Discard
			// both files and retry from 0.
			resp.Body.Close()
			cancel()
			if partSize > 0 {
				os.Remove(partPath)
				os.Remove(metaPath)
				lastErr = fmt.Errorf(
					"range not satisfiable; discarded .part/.meta")
				continue
			} // end:if partSize > 0
			return fmt.Errorf(
				"[!] ERROR: Failed to download file: HTTP status %s",
				resp.Status)
		default:
			resp.Body.Close()
			cancel()
			// 4xx (other than 416) is permanent; 5xx may be transient.
			if resp.StatusCode >= 400 && resp.StatusCode < 500 {
				return fmt.Errorf(
					"[!] ERROR: Failed to download file: HTTP status %s",
					resp.Status)
			} // end:if 4xx
			lastErr = fmt.Errorf("HTTP status %s", resp.Status)
			continue
		} // end:switch status

		// B11: open .part for writing at startOffset.
		out, err := openPartForWrite(partPath, startOffset)
		if err != nil {
			resp.Body.Close()
			cancel()
			return fmt.Errorf("[!] ERROR: %v", err)
		} // end:if openPartForWrite error

		// B1: per-read idle watchdog.
		watchdog := time.AfterFunc(defaultIdleTimeout, cancel)

		// Stream body.
		buf := make([]byte, 1024*1024*2) // 2 MB buffer
		fileSize := startOffset
		var bodyErr error
		var writeErr error

		for {
			n, rerr := resp.Body.Read(buf)
			if n > 0 {
				// B1: progress -> push the idle deadline out.
				watchdog.Reset(defaultIdleTimeout)
				if _, werr := out.Write(buf[:n]); werr != nil {
					writeErr = fmt.Errorf("write failed: %v", werr)
					break
				} // end:if write error
				fileSize += int64(n)
				// B7: dot per 2 MB boundary crossed.
				for fileSize >= nextDot {
					fmt.Print(".")
					nextDot += 1024 * 1024 * 2
				} // end:for dot
			} // end:if n > 0
			if rerr != nil {
				if errors.Is(rerr, io.EOF) {
					break
				} // end:if EOF
				if ctx.Err() != nil {
					bodyErr = fmt.Errorf(
						"stalled: no data for %v", defaultIdleTimeout)
				} else {
					bodyErr = fmt.Errorf("read failed: %v", rerr)
				} // end:if stalled
				break
			} // end:if rerr != nil
		} // end:for read loop

		watchdog.Stop()
		resp.Body.Close()

		// B8: local write failure is permanent.
		if writeErr != nil {
			out.Close()
			return fmt.Errorf("[!] ERROR: %v", writeErr)
		} // end:if writeErr

		// B11: read failure keeps .part (and .meta) for the next resume.
		if bodyErr != nil {
			out.Close()
			lastErr = bodyErr
			continue
		} // end:if bodyErr

		// B3: verify the advertised length actually arrived.
		if expectTotal >= 0 && fileSize != expectTotal {
			out.Close()
			lastErr = fmt.Errorf(
				"short read: got %d of %d bytes", fileSize, expectTotal)
			continue
		} // end:if short read

		// Success: close, rename atomically, remove sidecar.
		if cerr := out.Close(); cerr != nil {
			return fmt.Errorf("[!] ERROR: Failed to close file: %v", cerr)
		} // end:if close error
		if rerr := os.Rename(partPath, target); rerr != nil {
			return fmt.Errorf(
				"[!] ERROR: Failed to finalize file: %v", rerr)
		} // end:if rename error
		os.Remove(metaPath) // B12: sidecar no longer needed

		printDownloadStats(startTime, fileSize)
		return nil
	} // end:for attempt

	return fmt.Errorf("[!] ERROR: failed after %d attempts: %v",
		maxAttempts, lastErr)
} // end:downloadFile

// --------------------------------------------------------------------------
// effectiveTarget applies the trusted-symlink policy and returns the
// path that should receive the downloaded bytes (S1).
func effectiveTarget(fileName string) (string, error) {
	fi, err := os.Lstat(fileName)
	if os.IsNotExist(err) {
		return fileName, nil
	} // end:if not-exist
	if err != nil {
		return "", err
	} // end:if lstat error
	if fi.Mode()&os.ModeSymlink == 0 {
		return fileName, nil
	} // end:if not symlink
	if !symlinkOwnerTrusted(fi) {
		return "", errUntrustedSymlink
	} // end:if untrusted
	resolved, rerr := filepath.EvalSymlinks(fileName)
	if rerr != nil {
		return "", fmt.Errorf(
			"trusted symlink %q cannot be resolved: %v",
			fileName, rerr)
	} // end:if eval error
	return resolved, nil
} // end:effectiveTarget

// --------------------------------------------------------------------------
// symlinkOwnerTrusted reports whether following the symlink described
// by fi is allowed: the link's owner must be root or the invoking user.
// Default-deny on Windows (no UID model in stdlib). (S1.)
func symlinkOwnerTrusted(fi os.FileInfo) bool {
	if fi == nil || fi.Sys() == nil {
		return false
	} // end:if nil sys
	euid := os.Geteuid()
	if euid < 0 {
		return false // Windows: default-deny
	} // end:if no euid

	v := reflect.ValueOf(fi.Sys())
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return false
		} // end:if nil ptr
		v = v.Elem()
	} // end:if ptr
	if v.Kind() != reflect.Struct {
		return false
	} // end:if not struct

	uidField := v.FieldByName("Uid")
	if !uidField.IsValid() {
		return false
	} // end:if no Uid
	if uidField.Kind() < reflect.Uint || uidField.Kind() > reflect.Uint64 {
		return false
	} // end:if not uint
	uid := uidField.Uint()
	return uid == 0 || uid == uint64(euid)
} // end:symlinkOwnerTrusted

// --------------------------------------------------------------------------
// existingPartSize returns the size of partPath if it exists as a
// regular file, 0 if it does not exist, or an error if it exists as a
// non-regular file (symlink, directory, ...).
func existingPartSize(partPath string) (int64, error) {
	fi, err := os.Lstat(partPath)
	if os.IsNotExist(err) {
		return 0, nil
	} // end:if not-exist
	if err != nil {
		return 0, err
	} // end:if lstat error
	if !fi.Mode().IsRegular() {
		return 0, fmt.Errorf("refusing to use non-regular %q", partPath)
	} // end:if not regular
	return fi.Size(), nil
} // end:existingPartSize

// --------------------------------------------------------------------------
// openPartForWrite opens (or creates) the .part file and positions the
// write offset at startOffset.
//
//   startOffset == 0: create fresh with O_EXCL if possible; otherwise
//     truncate the existing regular .part and write from 0.
//   startOffset  > 0: open the existing regular .part, verify its size
//     is exactly startOffset, and seek to that offset.
//
// Non-regular .part files are refused (S1/B11).
func openPartForWrite(partPath string, startOffset int64) (*os.File, error) {
	if startOffset == 0 {
		f, err := os.OpenFile(partPath,
			os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
		if err == nil {
			return f, nil
		} // end:if created fresh
		if !os.IsExist(err) {
			return nil, err
		} // end:if not-exist
	} // end:if startOffset == 0

	fi, err := os.Lstat(partPath)
	if err != nil {
		return nil, err
	} // end:if lstat error
	if !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("refusing to use non-regular %q", partPath)
	} // end:if not regular
	if startOffset > 0 && fi.Size() != startOffset {
		return nil, fmt.Errorf(
			"part file size %d != expected %d", fi.Size(), startOffset)
	} // end:if size mismatch

	f, err := os.OpenFile(partPath, os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	} // end:if open error
	if startOffset == 0 {
		if err := f.Truncate(0); err != nil {
			f.Close()
			return nil, err
		} // end:if truncate error
	} // end:if truncate
	if _, err := f.Seek(startOffset, io.SeekStart); err != nil {
		f.Close()
		return nil, err
	} // end:if seek error
	return f, nil
} // end:openPartForWrite

// --------------------------------------------------------------------------
// readETag reads a persisted ETag from metaPath. Returns "" if the
// file does not exist, is empty, or contains an invalid value. Weak
// ETags (W/"...") are rejected: RFC 7233 forbids using them with
// If-Range. (B12.)
func readETag(metaPath string) (string, error) {
	data, err := os.ReadFile(metaPath)
	if err != nil {
		return "", err
	} // end:if read error
	etag := strings.TrimSpace(string(data))
	// Sanity: must look like a quoted ETag.
	if !strings.HasPrefix(etag, `"`) && !strings.HasPrefix(etag, `W/"`) {
		return "", fmt.Errorf("invalid ETag in %s: %q", metaPath, etag)
	} // end:if not quoted
	// RFC 7233 forbids weak validators in If-Range.
	if strings.HasPrefix(etag, "W/") {
		return "", nil
	} // end:if weak
	return etag, nil
} // end:readETag

// --------------------------------------------------------------------------
// writeETag persists an ETag to metaPath. Refuses invalid values
// (must be a quoted ETag) and non-regular target paths (S1 defence
// in depth). (B12.)
func writeETag(metaPath, etag string) error {
	if !strings.HasPrefix(etag, `"`) && !strings.HasPrefix(etag, `W/"`) {
		return fmt.Errorf("refusing to persist invalid ETag: %q", etag)
	} // end:if not quoted
	if fi, err := os.Lstat(metaPath); err == nil &&
		!fi.Mode().IsRegular() {
		return fmt.Errorf("refusing to write non-regular %q", metaPath)
	} // end:if non-regular
	return os.WriteFile(metaPath, []byte(etag+"\n"), 0644)
} // end:writeETag

// --------------------------------------------------------------------------
// parseContentRange parses "bytes START-END/TOTAL" or "bytes START-END/*".
// Returns (start, end, total, err). total == -1 means the server sent "*".
func parseContentRange(s string) (start, end, total int64, err error) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "bytes ") {
		return 0, 0, 0, fmt.Errorf("invalid Content-Range: %q", s)
	} // end:if prefix
	s = s[len("bytes "):]
	slash := strings.IndexByte(s, '/')
	if slash < 0 {
		return 0, 0, 0, fmt.Errorf("missing / in Content-Range: %q", s)
	} // end:if no slash
	rangePart := s[:slash]
	totalPart := s[slash+1:]

	dash := strings.IndexByte(rangePart, '-')
	if dash < 0 {
		return 0, 0, 0, fmt.Errorf("missing - in Content-Range: %q", s)
	} // end:if no dash
	start, err = strconv.ParseInt(rangePart[:dash], 10, 64)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid start: %v", err)
	} // end:if parse start
	end, err = strconv.ParseInt(rangePart[dash+1:], 10, 64)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid end: %v", err)
	} // end:if parse end
	if totalPart == "*" {
		total = -1
	} else {
		total, err = strconv.ParseInt(totalPart, 10, 64)
		if err != nil {
			return 0, 0, 0, fmt.Errorf("invalid total: %v", err)
		} // end:if parse total
	} // end:if total
	return start, end, total, nil
} // end:parseContentRange

// --------------------------------------------------------------------------
// printDownloadStats prints elapsed time, rate, and bytes transferred.
// (B10: rounded to ms to avoid nanosecond noise.)
func printDownloadStats(startTime time.Time, totalBytes int64) {
	elapsed := time.Since(startTime)
	mins := int(elapsed.Minutes())
	secs := elapsed.Seconds() - float64(mins)*60
	// MB/s
	downloadRate := float64(totalBytes) / elapsed.Seconds() / (1024 * 1024)

	if (mins == 0 && secs < 3) || (mins > 10) {
		// Original comment kept: 123us, 10min23s 1h23m ||
		// 65.98912ms (0.01 MB/s) 970 bytes OK!
		fmt.Printf(" -> %v (%.2f MB/s) %s bytes",
			elapsed.Round(time.Millisecond), downloadRate,
			formatThousands(totalBytes))
	} else {
		fmt.Printf(" -> %dm%.2fs (%.2f MB/s) %s bytes",
			mins, secs, downloadRate, formatThousands(totalBytes))
	} // end:if elapsed-format
} // end:printDownloadStats

// --------------------------------------------------------------------------
// formatThousands inserts dot separators for readability:
// 1234567 -> "1.234.567". Negative values keep the leading minus.
func formatThousands(value int64) string {
	formatted := strconv.FormatInt(value, 10)
	start := 0
	if strings.HasPrefix(formatted, "-") {
		start = 1
	} // end:if negative
	for position := len(formatted) - 3; position > start; position -= 3 {
		formatted = formatted[:position] + "." + formatted[position:]
	} // end:for insert dots
	return formatted
} // end:formatThousands

// --------------------------------------------------------------------------
// calculateMD5 calculates the MD5 hash of a file
// (S5: MD5 is cryptographically broken; kept for wire compatibility.)
func calculateMD5(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("[!] ERROR: Could not open file: %v", err)
	} // end:if open error
	defer file.Close()

	hash := md5.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf(
			"[!] ERROR: Could not calculate MD5: %v", err)
	} // end:if copy error

	return hex.EncodeToString(hash.Sum(nil)), nil
} // end:calculateMD5

// --------------------------------------------------------------------------
// displayHelp prints the usage instructions for the program
func displayHelp() {
	helpText := `
Usage:
  rupdater [option[s]]

Options:
  -d, --delete        Delete the md5sums.md5 file after processing.
  -k, --keep          Keep files that did not match the MD5 hash.
  -h, --help, -?      Show help message.
  -V, --version       Show program version.
  -u, --url URL       Specify the base URL. If not provided, the
                      default URL will be used.

Description:
  This program downloads files from the ROSE SWE download page listed in the
  remote file: md5sums.md5, verifies their MD5 checksums, and re-downloads
  (updates) files with mismatched MD5sums. You can choose to delete the
  md5sums.md5 file after processing, and optionally keep (broken) files even if
  their MD5 hash does not match.

  Failed downloads are resumed via HTTP Range on the next attempt or
  the next program run; the partial data is kept in a .part file.
  ETag validation via If-Range ensures a resume never appends bytes
  from a changed file on the server.

Exit Codes:
  0  - Success: The program completed without errors.
  1  - File Download Error: Unable to download the md5sums.md5 file.
  2  - File Open Error: Failed to open the md5sums.md5 file.
  3  - File Read Error: Error reading the md5sums.md5 file.
  4  - File Deletion Error: Error deleting the md5sums.md5 file after
       processing.
  5  - MD5 Mismatch Found: MD5 mismatches were detected and files were
       deleted (if applicable).

Example:
  rupdater -d -k

  Downloads files, keeps mismatched files, and deletes the md5sums.md5
  file when done.

  cfg2html mirroring:  rupdater  --url https://www.cfg2html.com/

  Mirrors instead the cfg2html website
`
	fmt.Println(strings.TrimSpace(helpText))
} // end:displayHelp

// --------------------------------------------------------------------------
// preprocessArgs maps long flags to short equivalents
func preprocessArgs() {
	// Map long flags to short flags
	argMap := map[string]string{
		"--help":    "-h",
		"--version": "-V",
		"--delete":  "-d",
		"--keep":    "-k",
		"--url":     "-u",
	} // end:argMap

	newArgs := []string{os.Args[0]}
	for _, arg := range os.Args[1:] {
		if mapped, exists := argMap[arg]; exists {
			newArgs = append(newArgs, mapped)
		} else {
			newArgs = append(newArgs, arg)
		} // end:if mapped
	} // end:for args
	os.Args = newArgs
} // end:preprocessArgs

// --------------------------------------------------------------------------
// parseMd5Line splits one md5sum line into (hash, file name, error).
// Accepts text mode "<hash>  <name>" and binary mode "<hash> *<name>".
// Returns ("", "", nil) for blank lines and comments. (Fixes B4.)
func parseMd5Line(line string) (string, string, error) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", nil
	} // end:if empty/comment
	if len(line) < 34 {
		return "", "", fmt.Errorf("line too short")
	} // end:if too short
	hash := line[:32]
	if !isHex32(hash) {
		return "", "", fmt.Errorf("invalid md5 hash: %q", hash)
	} // end:if not hex
	rest := line[32:]
	if !strings.HasPrefix(rest, " ") {
		return "", "", fmt.Errorf("missing separator after hash")
	} // end:if no separator
	// md5sum emits either "  <name>" (text) or " *<name>" (binary).
	rest = strings.TrimPrefix(rest, " ")
	rest = strings.TrimPrefix(rest, " ")
	rest = strings.TrimPrefix(rest, "*")
	if rest == "" {
		return "", "", fmt.Errorf("empty file name")
	} // end:if empty name
	return strings.ToLower(hash), rest, nil
} // end:parseMd5Line

// --------------------------------------------------------------------------
// isHex32 reports whether s is exactly 32 hexadecimal characters.
func isHex32(s string) bool {
	if len(s) != 32 {
		return false
	} // end:if length
	for i := 0; i < 32; i++ {
		c := s[i]
		if !((c >= '0' && c <= '9') ||
			(c >= 'a' && c <= 'f') ||
			(c >= 'A' && c <= 'F')) {
			return false
		} // end:if not hex char
	} // end:for i
	return true
} // end:isHex32

// --------------------------------------------------------------------------
// safeLocalName rejects file names that could escape the working
// directory. (Fixes S2.)
func safeLocalName(name string) error {
	if name == "" {
		return fmt.Errorf("empty file name")
	} // end:if empty
	if name == "." || name == ".." {
		return fmt.Errorf("invalid file name: %q", name)
	} // end:if dot
	if filepath.IsAbs(name) {
		return fmt.Errorf("absolute path not allowed: %q", name)
	} // end:if absolute
	// Cross-platform defense in depth: reject BOTH "/" and "\".
	if strings.ContainsAny(name, `/\`) {
		return fmt.Errorf(
			"file name must not contain a path separator: %q", name)
	} // end:if separator
	if strings.ContainsRune(name, 0) {
		return fmt.Errorf("file name must not contain NUL")
	} // end:if nul
	return nil
} // end:safeLocalName

// --------------------------------------------------------------------------
// main is a thin wrapper so deferred file.Close() in run() runs before
// os.Exit (os.Exit skips defers). (Fixes B5.)
func main() {
	os.Exit(run())
} // end:main

// --------------------------------------------------------------------------
// run executes the update flow and returns the process exit code.
// Splitting main into run() guarantees that defers in run execute even
// when we return an error exit code. (Fixes B5.)
func run() int {
	// Greeting line
	fmt.Println("---=[ rupdater by ROSE SWE, (c) 2024-2026 by Ralph Roth ]=------------------")
	// ./main.go:129:2: fmt.Println arg list ends with redundant newline
	fmt.Println("Automatic update program to always get the newest files from ROSE SWE!")
	fmt.Println("")

	// Preprocess args to handle long flags
	preprocessArgs()

	// Define command-line flags
	deleteFile := flag.Bool("d", false, "Delete the md5sums.md5 file after processing.")
	keepFiles := flag.Bool("k", false, "Keep files that did not match the MD5 hash.")
	showHelp := flag.Bool("h", false, "Show a detailed help message.")
	showVersion := flag.Bool("V", false, "Show the program version.")
	helpAlternative := flag.Bool("?", false, "Show a detailed help message.")
	// Base URL and downloaded file (hardcoded)
	baseURL := "http://rose-swe.bplaced.net/dl/"
	flag.StringVar(&baseURL, "u", baseURL, "Specify an other base URL")
	flag.Parse()

	// Handle version display
	if *showVersion {
		fmt.Printf("Version: %s\n", version)
		//fmt.Printf("Build: %", BuildDate)
		return 0
	} // end:if showVersion

	// Handle help display
	if *showHelp || *helpAlternative {
		displayHelp()
		return 0
	} // end:if showHelp

	// Ensure baseURL ends with "/"
	if !strings.HasSuffix(baseURL, "/") {
		baseURL += "/"
	} // end:if trailing slash
	fmt.Print("[Info] Mirroring now website:", baseURL)
	downloadedFile := "md5sums.md5"
	md5URL := baseURL + downloadedFile

	// Step 1: Download the md5sums.md5 file
	err := downloadFile(md5URL, downloadedFile)
	if err != nil {
		fmt.Printf("[!] Error downloading file: %s (%v)\n", md5URL, err)
		return 1 // Exit code 1: File download error
	} // end:if download error
	fmt.Println(" OK!")

	// Step 2: Open and parse the md5sums.md5 file line by line
	file, err := os.Open(downloadedFile)
	if err != nil {
		fmt.Printf("[!] Error opening file: %v\n", err)
		return 2 // Exit code 2: File open error
	} // end:if open error
	// B5: safe now -- run() returns before os.Exit, so this defer runs.
	defer file.Close()

	scanner := bufio.NewScanner(file)
	md5MismatchFound := false

	for scanner.Scan() {
		line := scanner.Text()
		// Step 3: Parse each line for MD5 hash and file name
		expectedMD5, fileName, perr := parseMd5Line(line)
		if perr != nil {
			fmt.Printf("[!] Invalid line format: %s (%v)\n", line, perr)
			continue
		} // end:if parse error
		if fileName == "" {
			continue // blank line or comment
		} // end:if skip

		// S2: reject path traversal before touching the filesystem.
		if err := safeLocalName(fileName); err != nil {
			fmt.Printf("[!] Refusing unsafe file name %q: %v\n",
				fileName, err)
			continue
		} // end:if unsafe name

		// S1: apply the trusted-symlink policy up front.
		target, terr := effectiveTarget(fileName)
		if terr != nil {
			fmt.Printf("[!] Skipping %s: %v\n", fileName, terr)
			continue
		} // end:if untrusted symlink

		// Step 4: Check if the file exists locally.
		// B6: any stat error other than "not exist" is a real problem.
		needDownload := true
		if _, statErr := os.Lstat(target); statErr == nil {
			needDownload = false
		} else if !os.IsNotExist(statErr) {
			fmt.Printf("[!] Error checking %s: %v\n", target, statErr)
			continue
		} // end:if stat
		if needDownload {
			// File does not exist locally, download it.
			fileDownloadURL := baseURL + fileName
			fmt.Printf("[New!] %s downloading...", fileName)

			err = downloadFile(fileDownloadURL, target)
			if err != nil {
				fmt.Printf("[!] Error downloading file %s: %v\n",
					fileName, err)
				continue
			} // end:if download error
			fmt.Printf(" OK!\n")
			//fmt.Printf("[!] File %s downloaded successfully.\n", fileName)  // debugging....
		} // end:if needDownload

		// Step 5: Calculate the MD5 hash of the file
		calculatedMD5, err := calculateMD5(target)
		if err != nil {
			fmt.Printf("[!] Error calculating MD5 for file %s: %v\n",
				fileName, err)
			continue
		} // end:if md5 error

		// Step 6: Compare the calculated MD5 with the expected MD5
		if calculatedMD5 != expectedMD5 {
			fmt.Printf("[!] WARNING1: MD5 mismatch for file %s. Expected: [%s], Got: [%s]\n",
				fileName, expectedMD5, calculatedMD5)
			md5MismatchFound = true

			// B11/B12: discard any stale .part and .meta so the
			// re-download does not resume from content that no longer
			// matches the server.
			os.Remove(target + ".part")
			os.Remove(target + ".part.meta")

			// Attempt to download the file again
			fmt.Printf("[!] Attempting to re-download file %s to resolve MD5 mismatch...", fileName)
			err = downloadFile(baseURL+fileName, target)
			if err != nil {
				fmt.Printf("\n[!] Error re-downloading file %s: %v\n",
					fileName, err)
				continue
			} // end:if re-download error
			fmt.Printf("\n[!] File %s re-downloaded successfully.\n",
				fileName)

			// Recalculate MD5 after re-downloading
			calculatedMD5, err = calculateMD5(target)
			if err != nil {
				fmt.Printf("[!] Error recalculating MD5 for file %s: %v\n",
					fileName, err)
				continue
			} // end:if md5 error

			// Compare again
			if calculatedMD5 != expectedMD5 {
				fmt.Printf("[!] WARNING2: MD5 mismatch still persists for file %s. Expected: [%s], Got: [%s]\n",
					fileName, expectedMD5, calculatedMD5)

				// Delete the mismatched file unless -k was given
				if !*keepFiles {
					err = os.Remove(target)
					if err != nil {
						fmt.Printf("[!] Error deleting mismatched file %s: %v\n",
							fileName, err)
						continue
					} // end:if remove error
					fmt.Printf("[!] Mismatched file %s deleted.\n",
						fileName)
				} // end:if !keepFiles
			} else {
				fmt.Printf("[!] File %s is now valid after re-downloading (MD5 matches)\n",
					fileName)
			} // end:if still mismatch
		} else {
			fmt.Printf("[!OK!] %s is valid (MD5 matches)\n", fileName)
		} // end:if md5 mismatch
	} // end:for scanner.Scan

	if err := scanner.Err(); err != nil {
		fmt.Printf("[!] Error reading file: %v\n", err)
		return 3 // Exit code 3: File reading error
	} // end:if scanner error

	// Step 7: Conditionally delete md5sums.md5 if -d was given.
	if *deleteFile {
		err = os.Remove(downloadedFile)
		if err != nil {
			fmt.Printf("[!] Error deleting file %s: [%v]\n",
				downloadedFile, err)
			return 4 // Exit code 4: File deletion error
		} // end:if remove error
		fmt.Printf("[Info] Downloaded file %s deleted successfully.\n",
			downloadedFile)
	} // end:if deleteFile

	// Step 8: Exit with unique return code if there was an MD5 mismatch.
	if md5MismatchFound && !*keepFiles {
		return 5 // Exit code 5: MD5 mismatches found
	} // end:if exit 5

	return 0 // Exit code 0: Success
} // end:run