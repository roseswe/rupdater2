
# Rupdater2 (ROSE SWE website updater program version 2)

[rupdater2](https://github.com/roseswe/rupdater2) is a simple Go-based program that downloads files from a remote server (the ROSE SWE download page), verifies their MD5 hashes, and re-downloads files with mismatches. Optionally, it can delete files that do not match the expected MD5 hash.

This is the successor of rupdater, a bash script https://github.com/roseswe/rupdater

## Overview

Currently, there is no integrated automated update mechanism for the programs from **ROSE SWE**. To address this, we provide a shell script, `rupdater`, to facilitate updates from the following sites:

- [rose.rult.at](http://rose.rult.at/)
- [cfg2html.com](http://www.cfg2html.com)

As rupdater (v1, shell script) has some shortcomings (Issue #2) we decided to program a complete new version in Go(lang) -> [rupdater2](https://github.com/roseswe/rupdater)

rupdater2 supports beside Linux also Windows environments, which the first version (rupdater) does not. Porting to other platforms like ARM64 etc. is possible.

## Features

- Downloads files listed in a remote `md5sums.md5` file.
- Verifies MD5 hashes of the downloaded files.
- Re-downloads files with mismatched MD5 hashes.
- Optionally deletes the file md5sums.md5 after processing.
- Supports cross-compilation for both Windows and Linux.

## Prerequisites for rebuilding

- [Go](https://golang.org/dl/) version 1.18 or higher installed on your machine. e.g. golang-1.18-go/jammy-updates,jammy-security,now 1.18.1-1ubuntu1.1 amd64 [installed,automatic] - tested successfully also with go 1.23
- `make` (optional, if using the Makefile for building).
- if you want to (re)create the distribution ZIP archive you need additional tools like UPX, gitchangelog, pandoc, goversioninfo, describe (rose_swe) etc.

## Installation

To get started with the project, clone the repository to your local machine:

```bash
git clone https://github.com/roseswe/rupdater2.git
cd rupdater2
```

### Building the Project

You can build the project for Windows and Linux (32-bit) by running the following `make` command, which will cross-compile the program for both platforms:

```bash
make
```

This will generate the following executables in the `build` directory:
- `build/rupdater.exe` (32-bit Windows executable)
- `build/rupdater64.exe` (64-bit Windows executable)
- `build/rupdater` (32-bit Linux executable, static linked, should run on all Linux platforms that provide a 32bit runtime)

### Manual Build (without Makefile)

Alternatively, if you don't have `make`, you can manually cross-compile the project for each platform using Go:

- **For Windows 32-bit**:
  ```bash
  GOOS=windows GOARCH=386 go build -o build/rupdater.exe main.go
  ```

- **For Linux 32-bit**:
  ```bash
  GOOS=linux GOARCH=386 go build -o build/rupdater main.go
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

   ---=[ rupdater by ROSE SWE, (c) 2024 by Ralph Roth ]=------------------
   Automatic update program to always get the newest files from ROSE SWE!

   Usage:
   rupdater [option[s]]

   Options:
   -d, --delete        Delete the md5sums.md5 file after processing.
   -k, --keep          Keep files that did not match the MD5 hash.
   -h, --help, -?      Show help message.
   -V, --version       Show program version.

   Description:
   This program downloads files from the ROSE SWE download page listed in the
   remote file: md5sums.md5, verifies their MD5 checksums, and re-downloads
   (updates) files with mismatched MD5sums. You can choose to delete the
   md5sums.md5 file after processing, and optionally keep (broken) files even if
   their MD5 hash does not match.

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

   ```
4. **Downloading/Mirroring www.cfg2html.com**

   All websites that have been created using ROSE_SWE's dir2html package can be mirrored:

   ```bash
   ./rupdater --url https://www.cfg2html.com
   ```

## Exit Codes

- `0`: Success - Program completed without errors.
- `1`: File Download Error - Unable to download the `md5sums.md5` file.
- `2`: File Open Error - Failed to open the `md5sums.md5` file.
- `3`: File Read Error - Error reading the `md5sums.md5` file.
- `4`: File Deletion Error - Error deleting the `md5sums.md5` file after processing.
- `5`: MD5 Mismatch Found - MD5 mismatches were detected, and files were deleted (if applicable).

## Contributing

Feel free to fork the repository and submit pull requests. If you have any suggestions or bug reports, open an issue.

### Issues

If you find any bugs or have feature requests, please open an issue on the GitHub repository.

## License

This project is open-source and available under the [MIT License](LICENSE).

// END // $Id: README.md,v 1.8 2024/10/23 09:49:26 ralph Exp $
