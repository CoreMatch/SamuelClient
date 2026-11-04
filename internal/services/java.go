package services

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// DetectJavaVersion runs `java -version` against javaPath and extracts the
// version string from its output. Java prints the version banner to stderr,
// so stderr is read first and stdout is used as a fallback.
func DetectJavaVersion(javaPath string) (string, error) {
	var stdout, stderr bytes.Buffer
	cmd := exec.Command(javaPath, "-version")
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("failed to run java -version: %w", err)
	}

	output := stderr.String()
	if output == "" {
		output = stdout.String()
	}

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, "version") {
			parts := strings.Split(line, "\"")
			if len(parts) >= 2 {
				return strings.TrimSpace(parts[1]), nil
			}
		}
	}

	return "Unknown", nil
}

// DetectSystemJava attempts to find the system Java executable, trying PATH
// first and then common installation locations on Linux and Windows. Returns
// an empty string when no Java can be found.
func DetectSystemJava() string {
	// Common Java locations on Linux
	linuxPaths := []string{
		"/usr/bin/java",
		"/usr/lib/jvm/java-17-openjdk-amd64/bin/java",
		"/usr/lib/jvm/java-11-openjdk-amd64/bin/java",
		"/usr/lib/jvm/java-21-openjdk-amd64/bin/java",
		"/usr/lib/jvm/default-java/bin/java",
		"/opt/java/openjdk/bin/java",
	}

	// Common Java locations on Windows
	windowsPaths := []string{
		"C:\\Program Files\\Java\\jdk17\\bin\\java.exe",
		"C:\\Program Files\\Java\\jdk11\\bin\\java.exe",
		"C:\\Program Files\\Java\\jdk21\\bin\\java.exe",
		"C:\\Program Files\\Java\\jre\\bin\\java.exe",
		"C:\\Program Files (x86)\\Java\\jre\\bin\\java.exe",
	}

	// Try to find java in PATH
	var stdout bytes.Buffer
	cmd := exec.Command("which", "java")
	cmd.Stdout = &stdout
	if err := cmd.Run(); err == nil {
		path := strings.TrimSpace(stdout.String())
		if path != "" {
			// Verify it's executable
			if _, err := exec.LookPath(path); err == nil {
				return path
			}
		}
	}

	// Check common Linux paths
	for _, path := range linuxPaths {
		if _, err := exec.LookPath(path); err == nil {
			return path
		}
	}

	// Check common Windows paths
	for _, path := range windowsPaths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	return ""
}
