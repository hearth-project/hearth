/*
Copyright 2026 The Hearth Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package utils

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2" // nolint:revive,staticcheck
)

const (
	defaultKindBinary  = "kind"
	defaultKindCluster = "kind"
	podmanTool         = "podman"
)

// Run executes a command from the repository root.
func Run(cmd *exec.Cmd) (string, error) {
	dir, err := GetProjectDir()
	if err != nil {
		return "", err
	}
	cmd.Dir = dir

	cmd.Env = append(os.Environ(), "GO111MODULE=on")
	command := strings.Join(cmd.Args, " ")
	_, _ = fmt.Fprintf(GinkgoWriter, "running: %q\n", command)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("%q failed with error %q: %w", command, string(output), err)
	}

	return string(output), nil
}

// LoadImageToKindClusterWithName loads a local image into the configured Kind cluster.
func LoadImageToKindClusterWithName(name string) error {
	cluster := defaultKindCluster
	if v, ok := os.LookupEnv("KIND_CLUSTER"); ok {
		cluster = v
	}
	kindBinary := defaultKindBinary
	if v, ok := os.LookupEnv("KIND"); ok {
		kindBinary = v
	}
	if os.Getenv("KIND_EXPERIMENTAL_PROVIDER") == podmanTool || os.Getenv("CONTAINER_TOOL") == podmanTool {
		dir, err := os.MkdirTemp("", "hearth-kind-image-")
		if err != nil {
			return fmt.Errorf("create image archive directory: %w", err)
		}
		defer func() { _ = os.RemoveAll(dir) }()

		tool := os.Getenv("CONTAINER_TOOL")
		if tool == "" {
			tool = podmanTool
		}
		archive := filepath.Join(dir, "image.tar")
		if _, err := Run(exec.Command(tool, "save", "--output", archive, name)); err != nil {
			return fmt.Errorf("save image %q: %w", name, err)
		}
		_, err = Run(exec.Command(kindBinary, "load", "image-archive", archive, "--name", cluster))
		return err
	}

	cmd := exec.Command(kindBinary, "load", "docker-image", name, "--name", cluster)
	_, err := Run(cmd)
	return err
}

// GetNonEmptyLines splits command output and removes empty lines.
func GetNonEmptyLines(output string) []string {
	var res []string
	elements := strings.SplitSeq(output, "\n")
	for element := range elements {
		if element != "" {
			res = append(res, element)
		}
	}

	return res
}

// GetProjectDir returns the repository root.
func GetProjectDir() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get current working directory: %w", err)
	}
	for dir := wd; ; dir = filepath.Dir(dir) {
		info, statErr := os.Stat(filepath.Join(dir, "go.mod"))
		if statErr == nil && !info.IsDir() {
			return dir, nil
		}
		if statErr != nil && !os.IsNotExist(statErr) {
			return "", fmt.Errorf("inspect repository root: %w", statErr)
		}
		if filepath.Dir(dir) == dir {
			return "", fmt.Errorf("find repository root from %q: go.mod not found", wd)
		}
	}
}
