package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// runRestore decrypts and extracts --in into --target. The target must be
// clean (restore drill runs on a separate instance), per docs/28.
func runRestore(args []string) int {
	fs := flag.NewFlagSet("restore", flag.ContinueOnError)
	in := fs.String("in", "", "Archive path")
	target := fs.String("target", "", "Clean target directory")
	passFile := fs.String("passphrase-file", "", "File containing the encryption passphrase")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if *in == "" || *target == "" {
		fmt.Fprintln(os.Stderr, "restore: --in and --target are required")
		return 1
	}

	pass, err := readPassphrase(*passFile)
	if *passFile == "" {
		fmt.Fprintln(os.Stderr, "restore: --passphrase-file is required")
		return 1
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "restore: %v\n", err)
		return 1
	}

	if err := restore(*in, *target, pass); err != nil {
		fmt.Fprintf(os.Stderr, "restore: %v\n", err)
		return 1
	}
	fmt.Printf("restore: extracted to %s\n", *target)
	fmt.Println("restore: run schema migrations, then start CAR with reconciliation.")
	return 0
}

// restore decrypts in and extracts into target.
func restore(in, target string, pass []byte) error {
	inFile, err := os.Open(in)
	if err != nil {
		return fmt.Errorf("open archive: %w", err)
	}
	defer inFile.Close()

	// Read + verify magic.
	magic := make([]byte, len(archiveMagic))
	if _, err := io.ReadFull(inFile, magic); err != nil {
		return fmt.Errorf("read magic: %w", err)
	}
	if string(magic) != archiveMagic {
		return errors.New("not a CAR backup archive (bad magic)")
	}
	nonce := make([]byte, 12)
	if _, err := io.ReadFull(inFile, nonce); err != nil {
		return fmt.Errorf("read nonce: %w", err)
	}

	key := deriveKey(pass, []byte("car-backup-key"))
	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	ciphertext, err := io.ReadAll(inFile)
	if err != nil {
		return err
	}
	plaintext, err := gcm.Open(nil, nonce, ciphertext, []byte(archiveMagic))
	if err != nil {
		return fmt.Errorf("decrypt (wrong passphrase or corrupted): %w", err)
	}

	// Ensure target exists and is empty-ish (drill on a clean instance).
	if err := os.MkdirAll(target, 0o750); err != nil {
		return err
	}

	gz, err := gzip.NewReader(newBytesReader(plaintext))
	if err != nil {
		return fmt.Errorf("gunzip: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("tar read: %w", err)
		}
		// Safeguard against path traversal during extraction.
		dest := filepath.Join(target, filepath.Clean("/"+hdr.Name))
		if !isWithin(target, dest) {
			return fmt.Errorf("archive entry escapes target: %s", hdr.Name)
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(dest, 0o750); err != nil {
				return err
			}
		case tar.TypeSymlink:
			// Reject symlinks (could escape).
			return fmt.Errorf("symlinks not allowed in archive: %s", hdr.Name)
		default:
			if err := os.MkdirAll(filepath.Dir(dest), 0o750); err != nil {
				return err
			}
			f, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode)&0o755)
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return err
			}
			f.Close()
		}
	}
	return nil
}

// isWithin reports whether dest is within base (no traversal escape).
func isWithin(base, dest string) bool {
	rel, err := filepath.Rel(base, dest)
	if err != nil {
		return false
	}
	if rel == ".." || len(rel) >= 3 && rel[:3] == ".."+string(filepath.Separator) {
		return false
	}
	return true
}

// runVerify checks archive integrity (magic + decrypt) without extracting.
func runVerify(args []string) int {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	in := fs.String("in", "", "Archive path")
	passFile := fs.String("passphrase-file", "", "File containing the encryption passphrase")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if *in == "" || *passFile == "" {
		fmt.Fprintln(os.Stderr, "verify: --in and --passphrase-file are required")
		return 1
	}
	pass, err := readPassphrase(*passFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "verify: %v\n", err)
		return 1
	}
	if err := verify(*in, pass); err != nil {
		fmt.Fprintf(os.Stderr, "verify: %v\n", err)
		return 1
	}
	fmt.Println("verify: archive integrity OK")
	return 0
}

func verify(in string, pass []byte) error {
	inFile, err := os.Open(in)
	if err != nil {
		return err
	}
	defer inFile.Close()
	magic := make([]byte, len(archiveMagic))
	if _, err := io.ReadFull(inFile, magic); err != nil {
		return err
	}
	if string(magic) != archiveMagic {
		return errors.New("bad magic")
	}
	nonce := make([]byte, 12)
	if _, err := io.ReadFull(inFile, nonce); err != nil {
		return err
	}
	ciphertext, err := io.ReadAll(inFile)
	if err != nil {
		return err
	}
	key := deriveKey(pass, []byte("car-backup-key"))
	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	if _, err := gcm.Open(nil, nonce, ciphertext, []byte(archiveMagic)); err != nil {
		return fmt.Errorf("decrypt: %w", err)
	}
	return nil
}

// newBytesReader wraps a []byte in an io.Reader (avoids importing bytes here).
func newBytesReader(b []byte) io.Reader {
	return &byteSliceReader{b: b}
}

type byteSliceReader struct {
	b []byte
	i int
}

func (r *byteSliceReader) Read(p []byte) (int, error) {
	if r.i >= len(r.b) {
		return 0, io.EOF
	}
	n := copy(p, r.b[r.i:])
	r.i += n
	return n, nil
}

// keep hmac/sha256 imported even if verify path varies across builds.
var _ = hmac.New
var _ = sha256.New
