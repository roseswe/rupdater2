# 🚀 rupdater2

## Rupdater2 (ROSE SWE website updater program version 2)

💡 [rupdater2](https://github.com/roseswe/rupdater2) is a simple Go-based program that downloads files from a remote server (the ROSE SWE download page or www.cfg2html.com), verifies their MD5 hashes, and re-downloads files with mismatches/don't exist yet. Optionally, it can delete files that do not match the expected MD5 hash.

> This is the successor of rupdater, a bash script https://github.com/roseswe/rupdater

## Overview

Currently, there is no integrated automated update mechanism for the programs from **ROSE SWE**. To address this, we had provided in the past a shell script, `rupdater`, to facilitate updates from the following sites:

- [ROSE SWE home page](http://rose-swe.bplaced.net/)
- [cfg2html.com](http://www.cfg2html.com)

>[!NOTE]
>> 🛠️ As rupdater (v1, shell script) has some shortcomings (see for example github Issue #2) we decided to program a complete new version in Go(lang) -> [rupdater2](https://github.com/roseswe/rupdater2)
>> ✨ rupdater2 supports beside Linux also Windows and MacOS environments, which the first version (rupdater) does not. Porting to other platforms and architectures like ARM64, RISC-V etc. are possible.

## Features

- Downloads files listed in a remote `md5sums.md5` file.
- Verifies MD5 hashes of the downloaded files.
- Re-downloads files with mismatched MD5 hashes.
- 🔄 **Automatic resume of interrupted downloads** — a flaky or slow connection no longer forces you to start over from the beginning. Partial data is kept and picked up again automatically, both within a single run and across program restarts.
- 🛡️ **Safe resume** — if the file on the server has changed since the partial download started, rupdater detects this and restarts cleanly instead of producing a corrupted file.
- 🧹 **Automatic cleanup** — partial data is removed once a download completes, and mismatched files are cleaned up unless you ask to keep them.
- Optionally deletes the file md5sums.md5 after processing.
- Supports cross-compilation for both Windows, WSL, MacOS and Linux. Other platforms on request!

## Special note about the macOS (Darwin) binary provided

>[!CAUTION]
> The Darwin64 binary included in this package **is not signed** by us (ROSE SWE) and is compressed with UPX to save space. Because of macOS security features, macOS Gatekeeper will quarantine it and refuse to launch it with a "damaged application" or "unidentified developer" message. If you trust this application and want to run it, you can do so.
>
> **Recommended one-time fix after unzipping:**
>
> ```sh
> xattr -d com.apple.quarantine rupdater-darwin64
> codesign --force --deep -s - rupdater-darwin64
> ```
>
> **Alternative (GUI):**
>
> - Go to `System Preferences > Security & Privacy`.
> - In the "General" tab, you should see a message about the blocked application. Click on "Open Anyway."
>
> **If you prefer to skip UPX entirely, build from source:**
>
> ```sh
> git clone https://github.com/roseswe/rupdater2
> cd rupdater2
> make build-darwin
> ```

## Prerequisites for rebuilding

- [Go](https://golang.org/dl/) version 1.18 or higher installed on your machine. e.g. golang-1.18-go/jammy-updates,jammy-security,now 1.18.1-1ubuntu1.1 amd64 [installed,automatic] - tested successfully also with go 1.23-1.26, 1.27
- `make` (optional, if using the Makefile for building).
- if you want to (re)create the distribution ZIP archive you need additional tools like UPX, gitchangelog, pandoc, goversioninfo, describe (rose_swe) etc.

## Installation

To get started with the project, clone the repository to your local machine:

```bash
git clone https://github.com/roseswe/rupdater2.git
cd rupdater2
```

### Building the Project

You can build the project for all supported platforms (Windows 32/64-bit, Linux 32/64-bit and macOS 64-bit) by running the following `make` command, which will cross-compile the program for each platform:

```bash
make
```

This will generate the following executables in the `build` directory:

- `build/rupdater32.exe` — 32-bit Windows executable, PE32 executable (console) Intel 80386
- `build/rupdater64.exe` — 64-bit Windows executable, PE32+ executable (console) x86-64
- `build/rupdater_i686` — 32-bit Linux executable, static linked, should run on all Linux platforms that provide a 32bit runtime
- `build/rupdater_amd64` — 64-bit Linux executable, ELF 64-bit LSB executable, x86-64, version 1 (SYSV)
- `build/rupdater-darwin64` — macOS: Mach-O 64-bit x86_64 executable, flags:<|DYLDLINK|PIE>

If you only need one platform, use the corresponding target:

```bash
make build-windows    # Windows 32+64-bit
make build-linux      # Linux 32+64-bit
make build-darwin     # macOS 64-bit
```

### Manual Build (without Makefile)

Alternatively, if you don't have `make`, you can manually cross-compile the project for each platform using Go. rupdater is pure Go, so no C toolchain is needed — a plain `go build` works:

- **For Windows 32-bit:**
  ```bash
  GOOS=windows GOARCH=386 go build -o build/rupdater32.exe .
  ```

- **For Windows 64-bit:**
  ```bash
  GOOS=windows GOARCH=amd64 go build -o build/rupdater64.exe .
  ```

- **For Linux 32-bit:**
  ```bash
  GOOS=linux GOARCH=386 go build -o build/rupdater_i686 .
  ```

- **For Linux 64-bit:**
  ```bash
  GOOS=linux GOARCH=amd64 go build -o build/rupdater_amd64 .
  ```

- **For macOS 64-bit:**
  ```bash
  GOOS=darwin GOARCH=amd64 go build -o build/rupdater-darwin64 .
  ```

## Usage

### Command-Line Options

- `-d`, `--delete`        Delete the `md5sums.md5` file after processing.
- `-k`, `--keep`          Keep files that did not match the MD5 hash.
- `-h`, `--help`, `-?`    Show the (detailed) help message.
- `-V`, `--version`       Show the program version.
- `-u`, `--url=URL`       Specify the base URL. If not provided, default URL will be used.

### Example Usage

1. **Download files, keep mismatched files, and delete the `md5sums.md5` file:**

   ```bash
   ./rupdater -d -k
   ```

2. **Check program version:**

   ```bash
   ./rupdater -V
   ```

3. **Show help message:**

   ```bash
   ./rupdater -h

   ---=[ rupdater by ROSE SWE, (c) 2024-2026 by Ralph Roth ]=------------------
   Automatic update program to always get the newest files from ROSE SWE!

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
   ```

4. **Downloading/Mirroring www.cfg2html.com**

   All websites that have been created using ROSE_SWE's dir2html package can be mirrored:

   ```bash
   ./rupdater --url https://www.cfg2html.com
   ```

## Automatic Resume and .part Files

Rupdater2 is designed to work reliably on slow or unstable connections. When a download is interrupted — because the connection dropped, the server timed out, or you hit Ctrl+C — the partial data is not thrown away.

- **Within a single run:** rupdater retries up to 5 times and continues from where it stopped, so a brief network hiccup no longer restarts a large download from scratch.
- **Across program runs:** if you stop the program or it gets killed, just run it again. Rupdater picks up the partial download on the next start.
- **Server changed the file?** If the file on the server has changed since the partial download began, rupdater detects this and restarts cleanly, rather than producing a corrupt file. You will see a message like `Server responded with full content (file changed or no Range support); restarting`.

Partial downloads are stored next to the target file with a `.part` extension (plus a small `.part.meta` sidecar for change detection) and are removed automatically once the download completes. If you ever want to force a clean restart, just delete the `.part` file (and its `.part.meta` companion if present) and run the program again.

>[!NOTE]
> Because the `.part` file lives in the current working directory, it inherits the permissions of that directory. For best protection against corruption or tampering (for example, by another user in a shared `/tmp` directory), run rupdater in a directory that only you can write to.

## Exit Codes

- `0`: Success - Program completed without errors.
- `1`: File Download Error - Unable to download the `md5sums.md5` file.
- `2`: File Open Error - Failed to open the `md5sums.md5` file.
- `3`: File Read Error - Error reading the `md5sums.md5` file.
- `4`: File Deletion Error - Error deleting the `md5sums.md5` file after processing.
- `5`: MD5 Mismatch Found - MD5 mismatches were detected, and files were deleted (if applicable).

## Error Messages running into so far

Or successfully handled by rupdater...

```
* [New!] rms_3.9.2.zip downloading....[!] Error downloading file rms_3.9.2.zip: [!] ERROR: Failed to read response body: unexpected EOF

[!] WARNING1: MD5 mismatch for file rms_3.9.2.zip. Expected: [e02a2e706bf61bd55c2bf54a9ab0b04c], Got: [74c95c5db6fc83433fff5c7f501b39e2]
[!] Attempting to re-download file rms_3.9.2.zip to resolve MD5 mismatch........
[!] File rms_3.9.2.zip re-downloaded successfully.
[!] File rms_3.9.2.zip is now valid after re-downloading (MD5 matches)
```

## Example for mismatching MD5 sum

```
$ ./rupdater_amd64
---=[ rupdater by ROSE SWE, (c) 2024-2026 by Ralph Roth ]=------------------
Automatic update program to always get the newest files from ROSE SWE!

[Info] Mirroring now website:http://rose-swe.bplaced.net/dl/ -> 57.96298ms (0.02 MB/s) 1.011 bytes OK!

[!OK!] MemScan-DOS_51-5.zip is valid (MD5 matches)
[New!] MPScan_MP_6.6.zip downloading....... -> 0m33.44s (0.25 MB/s) 8.757.746 bytes OK!
[!OK!] MPScan_MP_6.6.zip is valid (MD5 matches)

[!OK!] rms_MP_5.12.zip is valid (MD5 matches)
[New!] RMS_NG_MP-1.5.zip downloading............................................................................................................... -> 14m28.554122795s (0.25 MB/s) 227.385.908 bytes OK!
[!OK!] RMS_NG_MP-1.5.zip is valid (MD5 matches)
[!OK!] rPCP-DOS_Rel-23.7z is valid (MD5 matches)
[!OK!] rupdater2_MP_202607.zip is valid (MD5 matches)
[!OK!] rUPP_Rel-049.7z is valid (MD5 matches)
[New!] rvct_MP-Rel-083.7z downloading.......... -> 0m58.79s (0.25 MB/s) 15.398.265 bytes OK!
[!] WARNING1: MD5 mismatch for file rvct_MP-Rel-083.7z. Expected: [d41d8cd98f00b204e9800998ecf8427e], Got: [e69645918dcf0fcab13f4f4188c122ee]
[!] Attempting to re-download file rvct_MP-Rel-083.7z to resolve MD5 mismatch.......... -> 0m58.78s (0.25 MB/s) 15.398.265 bytes
[!] File rvct_MP-Rel-083.7z re-downloaded successfully.
[!] WARNING2: MD5 mismatch still persists for file rvct_MP-Rel-083.7z. Expected: [d41d8cd98f00b204e9800998ecf8427e], Got: [e69645918dcf0fcab13f4f4188c122ee]
[!] Mismatched file rvct_MP-Rel-083.7z deleted.
[!OK!] ViBa_DOS_3-43.zip is valid (MD5 matches)
[!OK!] VirScanPlus-DOS_32-7.zip is valid (MD5 matches)

```

## WSL

Both rupdater2 Linux binaries tested successfully under Windows Subsystem Linux (WSL)

## Contributing

Feel free to fork the repository and submit pull requests. If you have any suggestions or bug reports, open an issue.

### Issues

If you find any bugs or have feature requests, please open an issue on the GitHub repository.

## License

This project is open-source and available under the [MIT License](LICENSE).

// END // \$Id: README.md,v 1.23 2026/11/09 13:05:08 ralph Exp $