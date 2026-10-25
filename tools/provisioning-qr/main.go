package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	inputPath := flag.String("input", "", "verified public provisioning payload JSON")
	outputPath := flag.String("output", "", "PNG output path")
	flag.Parse()

	if *inputPath == "" || *outputPath == "" {
		fmt.Fprintln(os.Stderr, "both -input and -output are required")
		os.Exit(2)
	}

	payload, err := readPayload(*inputPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	image, err := renderAndVerify(payload)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	inputAbs, err := filepath.Abs(*inputPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "resolve input path:", err)
		os.Exit(1)
	}
	outputAbs, err := filepath.Abs(*outputPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "resolve output path:", err)
		os.Exit(1)
	}
	if filepath.Clean(inputAbs) == filepath.Clean(outputAbs) {
		fmt.Fprintln(os.Stderr, "input and output paths must differ")
		os.Exit(1)
	}
	if err := writeNewPNG(outputAbs, image); err != nil {
		fmt.Fprintln(os.Stderr, "write QR image:", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stdout, "Verified provisioning QR written to %s\n", outputAbs)
}

// Exclusive creation also refuses existing symlinks/hard links and preserves inputs.
func writeNewPNG(path string, image []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := file.Write(image); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	return file.Close()
}
