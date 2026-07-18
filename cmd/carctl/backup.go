package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Archive header magic, used to detect a malformed/unencrypted input early.
const archiveMagic = "CARBACK01"

// runBackup creates an encrypted archive of --source (database + artifacts)
// at --out. The passphrase is read from a file (never the command line,
// never logged).
func runBackup(args []string) int {
	fs := flag.NewFlagSet("backup", flag.ContinueOnError)
	source := fs.String("source", "", "Directory containing car.db and artifacts/")
	out := fs.String("out", "", "Output archive path")
	passFile := fs.String("passphrase-file", "", "File containing the encryption passphrase")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if *source == "" || *out == "" {
		fmt.Fprintln(os.Stderr, "backup: --source and --out are required")
		return 1
	}
	if *passFile == "" {
		fmt.Fprintln(os.Stderr, "backup: --passphrase-file is required")
		return 1
	}

	pass, err := readPassphrase(*passFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "backup: %v\n", err)
		return 1
	}

	if err := backup(*source, *out, pass); err != nil {
		fmt.Fprintf(os.Stderr, "backup: %v\n", err)
		return 1
	}
	fmt.Printf("backup: wrote %s\n", *out)
	return 0
}

// backup archives source into out, encrypted with AES-256-GCM keyed from pass.
func backup(source, out string, pass []byte) error {
	outFile, err := os.Create(out)
	if err != nil {
		return fmt.Errorf("create archive: %w", err)
	}
	defer outFile.Close()

	// Derive key from passphrase with HKDF-ish (HMAC-SHA256 with salt).
	key := deriveKey(pass, []byte("car-backup-key"))
	nonce := make([]byte, 12) // GCM standard nonce size
	if _, err := rand.Read(nonce); err != nil {
		return fmt.Errorf("nonce: %w", err)
	}

	// Write header: magic + nonce (plaintext; non-secret).
	if _, err := outFile.Write([]byte(archiveMagic)); err != nil {
		return err
	}
	if _, err := outFile.Write(nonce); err != nil {
		return err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}

	// Tar+gzip the plaintext, then encrypt into the archive.
	pr, pw := io.Pipe()
	go func() {
		err := tarGzipDir(source, pw)
		pw.CloseWithError(err)
	}()

	enc := gcm.Seal(nil, nonce, nil, nil) // empty AD
	// We need to stream; GCM is not a stream cipher, so we read all plaintext
	// then seal. Acceptable for homelab DB size. For very large artifact dirs
	// a chunked scheme would be needed.
	plaintext, err := io.ReadAll(pr)
	if err != nil {
		return fmt.Errorf("read archive stream: %w", err)
	}
	ciphertext := gcm.Seal(nil, nonce, plaintext, []byte(archiveMagic))
	if _, err := outFile.Write(ciphertext); err != nil {
		return err
	}
	_ = enc // (header carries the GCM tag implicitly via ciphertext)
	return nil
}

// tarGzipDir writes a gzipped tar of dir to w.
func tarGzipDir(dir string, w io.Writer) error {
	gz := gzip.NewWriter(w)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()

	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		hdr.Name = rel
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(tw, f)
		return err
	})
}

// deriveKey produces a 32-byte key from a passphrase and salt using
// HMAC-SHA256 in an HKDF-extract-like construction. Sufficient for homelab
// backup encryption; production may use scrypt/argon2.
func deriveKey(pass, salt []byte) []byte {
	mac := hmac.New(sha256.New, salt)
	mac.Write(pass)
	prk := mac.Sum(nil)
	// Expand to 32 bytes.
	exp := hmac.New(sha256.New, prk)
	exp.Write([]byte("car-backup-expand"))
	exp.Write([]byte{0x01})
	return exp.Sum(nil)
}

// readPassphrase reads the first line of a file, trimmed.
func readPassphrase(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read passphrase: %w", err)
	}
	// Trim trailing newline; keep the rest (passphrases may contain spaces).
	for len(data) > 0 && (data[len(data)-1] == '\n' || data[len(data)-1] == '\r') {
		data = data[:len(data)-1]
	}
	if len(data) < 8 {
		return nil, errors.New("passphrase too short (min 8 chars)")
	}
	return data, nil
}
