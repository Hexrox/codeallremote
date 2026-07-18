// Package main implements carctl, the CAR control CLI.
//
// Currently supports backup and restore drills (M3-03): encrypted backup of
// the database and artifacts, and restore onto a clean instance.
package main

import (
	"flag"
	"fmt"
	"os"
)

var version = "dev"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	showVersion := false
	fs := flag.NewFlagSet("carctl", flag.ContinueOnError)
	fs.BoolVar(&showVersion, "version", false, "Show version")
	_ = fs.Parse(os.Args[1:2])
	if showVersion {
		fmt.Printf("carctl %s\n", version)
		os.Exit(0)
	}

	switch os.Args[1] {
	case "backup":
		code := runBackup(os.Args[2:])
		os.Exit(code)
	case "restore":
		code := runRestore(os.Args[2:])
		os.Exit(code)
	case "verify":
		code := runVerify(os.Args[2:])
		os.Exit(code)
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `carctl — CAR control CLI

Usage:
  carctl backup   --source <dir> --out <file> [--passphrase-file <file>]
  carctl restore  --in <file> --target <dir> [--passphrase-file <file>]
  carctl verify   --in <file> [--passphrase-file <file>]
  carctl --version

Backup encrypts the SQLite database and artifacts directory into a single
archive. Restore reverses it onto a clean target. Verify checks archive
integrity without extracting.

The backup set MUST contain database + event journal + approval audit +
artifact metadata, restored together. RPO/RTO assumptions are recorded in
docs/28-disaster-recovery.md.`)
}
