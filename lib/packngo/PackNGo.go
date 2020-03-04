/*
Package packngo will pack, compress and encrypt any type of executable.
*/
package packngo

import (
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

const offsetPlaceholder = `"9999999"`
const depNamePlaceholder = `"DEPNAME1"`
const depSizePlaceholder = `"DEPSIZE2"`
const depElfPlaceholder = `"DEPELF3"`
const depBFDPlaceholder = "[]float64{1, 2, 3, 4}"
const launcherFile = "/tmp/launcher.go"

func cleanup() {
	fmt.Printf(ErrorColor, "\t\t[ ERR ]\n")
	fmt.Print(" → Cleaning up...")

	// remove unused file
	ExecCommand("rm", []string{"-f", launcherFile})
	fmt.Printf(SuccessColor, "\t\t\t[ OK ]\n")
}

func trap() {
	// Prepare to intercept SIGTERM
	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		cleanup()
		os.Exit(1)
	}()
}

// PackNGo will Encrypt and pack the payload for a secure execution
func PackNGo(infile string, offset int64, outfile string, dependency string, compress bool) {

    trap()

	fmt.Print(" → Randomizing offset...")

	// get the current script path
	selfPath := filepath.Dir(os.Args[0])

	// declare outfile as original filename + .enc
	if len(outfile) <= 0 {
		outfile = infile + ".enc"
	}

	// ------------------------------------------------------------------------
	// offset Hysteresis, this will prevent easy key retrieving
	rand.Seed(time.Now().UTC().UnixNano())
	offset = offset + (rand.Int63n(4096-128) + 128)

	fmt.Printf(SuccessColor, "\t\t[ OK ]\n")
	// ------------------------------------------------------------------------

	// ------------------------------------------------------------------------
	// Register Dependency to try and bypass any tampering on dependent
	// packages
	fmt.Print(" → Registering Dependencies...")

	// ------------------------------------------------------------------------
	// Register eventual dependency passed by cli
	// If a dependency check is present, register it.
	if dependency != "" {
		RegisterDependency(dependency)
	} else {
		// in case of missing dependency add an empty variable for BFD
		Secrets[depBFDPlaceholder] = []string{"[]int64[]", "leaveBFD"}
	}
	fmt.Printf(SuccessColor, "\t\t[ OK ]\n")
	// ------------------------------------------------------------------------

	// ------------------------------------------------------------------------
	// Create the launcher program starting from our stub
	fmt.Print(" → Creating Launcher Stub...")

	// add offset to the secrets!
	Secrets[offsetPlaceholder] = []string{fmt.Sprintf("%d", offset),
		GenerateTyposquatName()}

	// copy the stub from where to start.
	launcherStub, _ := base64.StdEncoding.DecodeString(LauncherStub)
	err := ioutil.WriteFile(launcherFile, launcherStub, 0644)
	if err != nil {
		fmt.Printf(ErrorColor, "\t\t[ ERR ]\n")
		println(fmt.Sprintf("failed writing to file: %s", err))
		cleanup()
		os.Exit(1)
	}
	fmt.Printf(SuccessColor, "\t\t[ OK ]\n")
	// ------------------------------------------------------------------------

	// ------------------------------------------------------------------------
	// Obfuscate the launcher
	fmt.Print(" → Obfuscating Launcher Stub...")
	err = ObfuscateLauncher(launcherFile)
	if err != nil {
		fmt.Printf(ErrorColor, "\t\t[ ERR ]\n")
		println(fmt.Sprintf("failed obfuscating file file: %s", err))
		cleanup()
		os.Exit(1)
	}
	fmt.Printf(SuccessColor, "\t\t[ OK ]\n")
	// ------------------------------------------------------------------------


	// ------------------------------------------------------------------------
	// compile the launcher binary
	fmt.Print(" → Compiling Launcher...")

	gopath, _ := os.LookupEnv("GOPATH")
	var flags []string
	os.Setenv("CGO_ENABLED", "0")
	flags = []string{"build", "-a",
		"-gcflags=-N",
		"-gcflags=-nolocalimports",
		"-gcflags=-trimpath=" + selfPath,
		"-asmflags=-trimpath=" + selfPath,
		"-gcflags=-trimpath=" + gopath + "/src/",
		"-asmflags=-trimpath=" + gopath + "/src/",
		"-ldflags=-extldflags=-static",
		"-ldflags=-s",
		"-ldflags=-w",
	}
	flags = append(flags, "-o")
	flags = append(flags, outfile)
	flags = append(flags, launcherFile)
	if ExecCommand("go", flags) {
		fmt.Printf(SuccessColor, "\t\t[ OK ]\n")
	} else {
		fmt.Printf(ErrorColor, "\t\t[ ERR ]\n")
		ExecCommand("rm", []string{"-f", outfile})
		cleanup()
		os.Exit(1)
	}
	// ------------------------------------------------------------------------

	// ------------------------------------------------------------------------
	// Strip File of excess headers
	fmt.Print(" → Stripping Launcher...")
	if StripFile(outfile) {
		fmt.Printf(SuccessColor, "\t\t[ OK ]\n")
	} else {
		fmt.Printf(ErrorColor, "\t\t[ ERR ]\n")
		ExecCommand("rm", []string{"-f", outfile})
		cleanup()
		os.Exit(1)
	}
	// ------------------------------------------------------------------------

	// ------------------------------------------------------------------------
	// Compress File of occupy less space
	// Then remove UPX headers from file.
	fmt.Print(" → Compressing Launcher...")
	if compress {
		if ExecCommand("upx", []string{outfile}) &&
			StripUPXHeaders(outfile) {
			fmt.Printf(SuccessColor, "\t\t[ OK ]\n")
		} else {
			fmt.Printf(ErrorColor, "\t\t[ ERR ]\n")
			ExecCommand("rm", []string{"-f", outfile})
			cleanup()
			os.Exit(1)
		}
	} else {
		fmt.Printf(WarningColor, "\t\t[ SKIPPING ]\n")
	}
	// ------------------------------------------------------------------------

	// ------------------------------------------------------------------------
	// Remove unused file
	fmt.Print(" → Cleaning up...")

	if ExecCommand("rm", []string{"-f", launcherFile}) {
		fmt.Printf(SuccessColor, "\t\t\t[ OK ]\n")
	} else {
		fmt.Printf(ErrorColor, "\t\t\t[ ERR ]\n")
		ExecCommand("rm", []string{"-f", outfile})
		os.Exit(1)
	}
	// ------------------------------------------------------------------------

	// read compiled file
	encFile, err := os.OpenFile(outfile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Printf(ErrorColor, "\t\t[ ERR ]\n")
		println(fmt.Sprintf("failed writing to file: %s", err))
		os.Exit(1)
	}
	defer encFile.Close()
	encFileStat, _ := encFile.Stat()
	encFileSize := encFileStat.Size()

	// ------------------------------------------------------------------------
	// Input validation
	fmt.Print(" → Verifying input offset...")

	// Ensure input offset is valid comared to compiled file size!
	if offset <= encFileSize {
		ExecCommand("rm", []string{"-f", outfile})
		fmt.Printf(ErrorColor, "\t[ ERR ]\n")
		println("ERROR! Calculated offset is lower than launcher size: " +
			fmt.Sprintf("offset=%d, filesize=%d", offset, encFileSize))
		os.Exit(1)
	}
	fmt.Printf(SuccessColor, "\t[ OK ]\n")
	// ------------------------------------------------------------------------


	// ------------------------------------------------------------------------
	// Pre-Payload Garbage
	// calculate where to put garbage and where to put the payload
	fmt.Print(" → Adding garbage...")

	blockCount := offset - encFileSize
	// append randomness to the runner itself
	_, err = encFile.WriteString(GenerateRandomGarbage(blockCount))
	if err != nil {
		fmt.Printf(ErrorColor, "\t\t\t[ ERR ]\n")
		println(fmt.Sprintf("failed writing to file: %s", err))
		os.Exit(1)
	}
	fmt.Printf(SuccessColor, "\t\t\t[ OK ]\n")
	// ------------------------------------------------------------------------


	// ------------------------------------------------------------------------
	// Encryption and compression of the payload
	// get file to encrypt argument
	fmt.Print(" → Reading payload...")

	byteContent, err := ioutil.ReadFile(infile) // just pass the file name
	if err != nil {
		fmt.Printf(ErrorColor, "\t\t\t[ ERR ]\n")
		println(fmt.Sprintf("failed reading file: %s", err))
		os.Exit(1)
	}
	content := string(byteContent)

	// plaintext content
	plaintext := []byte(base64.StdEncoding.EncodeToString([]byte(content)))

	fmt.Printf(SuccessColor, "\t\t\t[ OK ]\n")
	// ------------------------------------------------------------------------

	fmt.Print(" → Compressing payload...")

	// GZIP before encrypt
	plaintext = GzipContent(plaintext)
	fmt.Printf(SuccessColor, "\t\t[ OK ]\n")
	// ------------------------------------------------------------------------

	fmt.Print(" → Encrypting payload...")

	// encrypt aes256-gcm
	ciphertext, err := EncryptAESReversed(plaintext, outfile)
	if err != nil {
		fmt.Printf(ErrorColor, "\t\t[ ERR ]\n")
		println(fmt.Sprintf("failed encrypting file: %s", err))
		os.Exit(1)
	}

	// append payload to the runner itself
	_, err = encFile.WriteString(ciphertext)
	if err != nil {
		fmt.Printf(ErrorColor, "\t\t[ ERR ]\n")
		println(fmt.Sprintf("failed writing to file: %s", err))
		os.Exit(1)
	}
	fmt.Printf(SuccessColor, "\t\t[ OK ]\n")
	// ------------------------------------------------------------------------


	// ------------------------------------------------------------------------
	// Post-Payload Garbage
	// calculate final padding
	fmt.Print(" → Adding garbage to payload...")

	finalPaddingArray := make([]byte, binary.MaxVarintLen64)
	n := binary.PutVarint(finalPaddingArray, offset)
	finalPaddingB := finalPaddingArray[:n]
	// change endianess to every byte composing
	// the offset
	for i := range finalPaddingB {
		finalPaddingB[i] = ReverseByte(finalPaddingB[i])
	}
	finalPadding, _ := binary.Varint(finalPaddingB)
	// and ensure it is positive!
	if finalPadding < 0 {
		finalPadding = finalPadding * -1
	}

	// append random garbage equal to bit-reverse of the offset
	// at the end of the payload
	_, err = encFile.WriteString(GenerateRandomGarbage(finalPadding))
	if err != nil {
		fmt.Printf(ErrorColor, "\t\t[ ERR ]\n")
		println(fmt.Sprintf("failed writing to file: %s", err))
		os.Exit(1)
	}
	fmt.Printf(SuccessColor, "\t\t[ OK ]\n")
	// ------------------------------------------------------------------------

}
