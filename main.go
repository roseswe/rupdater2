/* 	(c) 2020-2024 by ROSE_SWE, Ralph Roth
	https://github.com/roseswe/rupdater2
 */
//go:generate goversioninfo -icon=main.ico -manifest=goversioninfo.exe.manifest
package main

import (
	"bufio"
	"crypto/md5"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

// Program version - see what(1) or mywhat
const version = "@(#)$Id: main.go,v 1.7 2024/10/23 09:49:27 ralph Exp $"

// var BuildDate string // This will be populated during the build

// downloadFile downloads a file from the given URL and saves it as the given file name
func downloadFile(url, fileName string) error {
	// Get the file from the URL
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("[!] ERROR: Failed to download file: %v", err)
	}
	defer resp.Body.Close()

	// Check HTTP status code
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("[!] ERROR: Failed to download file: HTTP status %s", resp.Status)
	}

	// Create a local file to store the downloaded content
	out, err := os.Create(fileName)
	if err != nil {
		return fmt.Errorf("[!] ERROR: Failed to create file: %v", err)
	}
	defer out.Close()

	// Write the body to the local file
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return fmt.Errorf("[!] ERROR: Failed to write to file: %v", err)
	}
	return nil
}

// calculateMD5 calculates the MD5 hash of a file
func calculateMD5(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("[!] ERROR: Could not open file: %v", err)
	}
	defer file.Close()

	hash := md5.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("[!] ERROR: Could not calculate MD5: %v", err)
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

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
  -u, --url=URL       Specify the base URL. If not provided, default URL
                      will be used.

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

  cfg2html mirroring:  rupdater  --url https://www.cfg2html.com/

  Mirrors instead the cfg2html website
`
	fmt.Println(strings.TrimSpace(helpText))
}

// preprocessArgs maps long flags to short equivalents
func preprocessArgs() {
	// Map long flags to short flags
	argMap := map[string]string{
		"--help":    "-h",
		"--version": "-V",
		"--delete":  "-d",
		"--keep":    "-k",
		"--url":     "-u",
	}

	newArgs := []string{os.Args[0]}
	for _, arg := range os.Args[1:] {
		if mapped, exists := argMap[arg]; exists {
			newArgs = append(newArgs, mapped)
		} else {
			newArgs = append(newArgs, arg)
		}
	}
	os.Args = newArgs
}

func main() {
	// Greeting line
	fmt.Println("---=[ rupdater by ROSE SWE, (c) 2024 by Ralph Roth ]=------------------")
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
		os.Exit(0)
	}

	// Handle help display
	if *showHelp || *helpAlternative {
		displayHelp()
		os.Exit(0)
	}

	// Ensure baseURL ends with "/"
	if !strings.HasSuffix(baseURL, "/") {
		baseURL += "/"
	}
	fmt.Println("[Info] Mirroring now website:", baseURL)
	downloadedFile := "md5sums.md5"
	md5URL := baseURL + downloadedFile

	// Step 1: Download the md5sums.md5 file
	err := downloadFile(md5URL, downloadedFile)
	if err != nil {
		fmt.Printf("[!] Error downloading file: %s (%v)\n", md5URL, err)
		os.Exit(1) // Exit code 1: File download error
	}

	// Step 2: Open and parse the md5sums.md5 file line by line
	file, err := os.Open(downloadedFile)
	if err != nil {
		fmt.Printf("[!] Error opening file: %v\n", err)
		os.Exit(2) // Exit code 2: File open error
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	md5MismatchFound := false

	for scanner.Scan() {
		line := scanner.Text()
		// Step 3: Parse each line for MD5 hash and file name
		parts := strings.Fields(line)
		if len(parts) != 2 {
			fmt.Printf("[!] Invalid line format: %s\n", line)
			continue
		}

		expectedMD5 := parts[0]
		fileName := parts[1]

		// Step 4: Check if the file exists locally
		if _, err := os.Stat(fileName); os.IsNotExist(err) {
			// File does not exist locally, download it from the base URL
			fileDownloadURL := baseURL + fileName
			fmt.Printf("[New!] %s downloading... ", fileName)

			err = downloadFile(fileDownloadURL, fileName)
			if err != nil {
				fmt.Printf("[!] Error downloading file %s: %v\n", fileName, err)
				continue
			} else {
				fmt.Printf("OK!\n")
			}
			//fmt.Printf("[!] File %s downloaded successfully.\n", fileName)  // debugging....
		}

		// Step 5: Calculate the MD5 hash of the file
		calculatedMD5, err := calculateMD5(fileName)
		if err != nil {
			fmt.Printf("[!] Error calculating MD5 for file %s: %v\n", fileName, err)
			continue
		}

		// Step 6: Compare the calculated MD5 with the expected MD5
		if calculatedMD5 != expectedMD5 {
			fmt.Printf("[!] WARNING1: MD5 mismatch for file %s. Expected: [%s], Got: [%s]\n", fileName, expectedMD5, calculatedMD5)
			md5MismatchFound = true

			// Attempt to download the file again
			fmt.Printf("[!] Attempting to re-download file %s to resolve MD5 mismatch...\n", fileName)
			err = downloadFile(baseURL+fileName, fileName)
			if err != nil {
				fmt.Printf("[!] Error re-downloading file %s: %v\n", fileName, err)
				continue
			}
			fmt.Printf("[!] File %s re-downloaded successfully.\n", fileName)

			// Recalculate MD5 after re-downloading
			calculatedMD5, err = calculateMD5(fileName)
			if err != nil {
				fmt.Printf("[!] Error recalculating MD5 for file %s: %v\n", fileName, err)
				continue
			}

			// Compare again
			if calculatedMD5 != expectedMD5 {
				fmt.Printf("[!] WARNING2: MD5 mismatch still persists for file %s. Expected: [%s], Got: [%s]\n", fileName, expectedMD5, calculatedMD5)

				// Delete the mismatched file unless the -k flag is set
				if !*keepFiles {
					err = os.Remove(fileName)
					if err != nil {
						fmt.Printf("[!] Error deleting mismatched file %s: %v\n", fileName, err)
						continue
					}
					fmt.Printf("[!] Mismatched file %s deleted.\n", fileName)
				}
			} else {
				fmt.Printf("[!] File %s is now valid after re-downloading (MD5 matches)\n", fileName)
			}
		} else {
			fmt.Printf("[!OK!] %s is valid (MD5 matches)\n", fileName)
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Printf("[!] Error reading file: %v\n", err)
		os.Exit(3) // Exit code 3: File reading error
	}

	// Step 7: Conditionally delete the downloaded md5sums.md5 file if the -d flag is set
	if *deleteFile {
		err = os.Remove(downloadedFile)
		if err != nil {
			fmt.Printf("[!] Error deleting file %s: [%v]\n", downloadedFile, err)
			os.Exit(4) // Exit code 4: File deletion error
		} else {
			fmt.Printf("[Info] Downloaded file %s deleted successfully.\n", downloadedFile)
		}
	}

	// Step 8: Exit with unique return code if there was an MD5 mismatch
	if md5MismatchFound && !*keepFiles {
		os.Exit(5) // Exit code 5: MD5 mismatches found
	}

	os.Exit(0) // Exit code 0: Success
}
