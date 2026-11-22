package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"
)

type PackJavaResolver func(context.Context, int, PackProgress) (string, error)

// The runtime cache is shared by imports and launch preparation, and uses
// its own destinations so manually managed runtimes are left alone.
var modpackJavaGate = make(chan struct{}, 1)

func EnsureModpackJava(ctx context.Context, cache string, paths []string, major int, progress PackProgress) (string, error) {
	if major == 0 {
		major = 8 // Mojang's older manifests do not declare javaVersion.
	}
	name := fmt.Sprintf("Temurin Java %d", major)
	matches := func(path string) bool {
		probe, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		version, err := detectJavaVersionContext(probe, path)
		number := strings.TrimPrefix(version, "1.")
		number, _, _ = strings.Cut(number, ".")
		number, _, _ = strings.Cut(number, "-")
		actual, _ := strconv.Atoi(number)
		return err == nil && (actual == major || (major == 16 && actual == 17))
	}
	for _, path := range slices.Compact(slices.Clone(paths)) {
		if path != "" && matches(path) {
			return path, nil
		}
		if err := ctx.Err(); err != nil {
			return "", err
		}
	}
	if major == 16 {
		major = 17
		name = "Temurin Java 17"
	}
	select {
	case modpackJavaGate <- struct{}{}:
		defer func() { <-modpackJavaGate }()
	case <-ctx.Done():
		return "", ctx.Err()
	}
	parent := filepath.Join(cache, strconv.Itoa(major))
	rel := filepath.Join("bin", "java")
	switch runtime.GOOS {
	case "windows":
		rel = filepath.Join("bin", "java.exe")
	case "darwin":
		rel = filepath.Join("Contents", "Home", "bin", "java")
	}
	entries, err := os.ReadDir(parent)
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	for _, entry := range entries {
		java := filepath.Join(parent, entry.Name(), "runtime", rel)
		if entry.IsDir() && strings.HasPrefix(entry.Name(), "jre-") && matches(java) {
			return java, nil
		}
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if !supportedJavaRuntimeMajors[major] {
		return "", fmt.Errorf("no automatic Java runtime available for Java %d on this system", major)
	}
	if progress != nil {
		progress("java-download", name, 0, 0)
	}
	if err := os.MkdirAll(parent, 0755); err != nil {
		return "", err
	}
	slot, err := os.MkdirTemp(parent, "jre-")
	if err != nil {
		return "", err
	}
	success := false
	defer func() {
		if !success {
			_ = os.RemoveAll(slot)
		}
	}()
	installed, err := NewJavaRuntimeInstaller().Install(ctx, major, filepath.Join(slot, "runtime"), func(stage string, downloaded, total int64) {
		if progress != nil {
			progress(stage, name, downloaded, total)
		}
	})
	if err != nil {
		return "", fmt.Errorf("install Java %d: %w", major, err)
	}
	if !matches(installed.JavaPath) {
		return "", fmt.Errorf("installed Java %d cannot run on this system", major)
	}
	success = true
	return installed.JavaPath, nil
}
