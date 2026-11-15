package services

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEnsureAuthlibInjectorJar(t *testing.T) {
	dataDir := t.TempDir()
	swapDataDir(t, dataDir)

	path, err := EnsureAuthlibInjectorJar()
	if err != nil {
		t.Fatalf("EnsureAuthlibInjectorJar: %v", err)
	}
	if want := filepath.Join(dataDir, "authlib-injector.jar"); path != want {
		t.Fatalf("jar path = %q, want %q", path, want)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read extracted jar: %v", err)
	}
	if !bytes.Equal(content, authlibInjectorJar) {
		t.Fatal("extracted jar differs from the embedded bytes")
	}

	// An identical copy must not be rewritten: pin the mtime and check it
	// survives a second call (compare against the read-back value so the
	// filesystem's timestamp granularity cannot flake the assertion).
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if _, err := EnsureAuthlibInjectorJar(); err != nil {
		t.Fatalf("second call: %v", err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if !after.ModTime().Equal(before.ModTime()) {
		t.Errorf("identical jar was rewritten (mtime %v, want %v)", after.ModTime(), before.ModTime())
	}

	// A stale/corrupt copy is replaced with the embedded bytes.
	if err := os.WriteFile(path, []byte("stale"), 0644); err != nil {
		t.Fatalf("corrupt jar: %v", err)
	}
	if _, err := EnsureAuthlibInjectorJar(); err != nil {
		t.Fatalf("third call: %v", err)
	}
	content, err = os.ReadFile(path)
	if err != nil || !bytes.Equal(content, authlibInjectorJar) {
		t.Fatalf("corrupt jar was not restored (err=%v)", err)
	}
}

// The committed asset must be a real java agent jar matching the declared
// version — not a download error page or a truncated file.
func TestBundledAuthlibInjectorJarIsValidAgent(t *testing.T) {
	zr, err := zip.NewReader(bytes.NewReader(authlibInjectorJar), int64(len(authlibInjectorJar)))
	if err != nil {
		t.Fatalf("embedded asset is not a zip/jar: %v", err)
	}
	var manifest string
	for _, f := range zr.File {
		if f.Name != "META-INF/MANIFEST.MF" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open manifest: %v", err)
		}
		b, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("read manifest: %v", err)
		}
		manifest = string(b)
	}
	if !strings.Contains(manifest, "Premain-Class: moe.yushi.authlibinjector.Premain") {
		t.Errorf("manifest lacks the authlib-injector Premain-Class:\n%s", manifest)
	}
	if !strings.Contains(manifest, "Implementation-Version: "+AuthlibInjectorVersion) {
		t.Errorf("manifest version does not match AuthlibInjectorVersion %q:\n%s", AuthlibInjectorVersion, manifest)
	}
}
