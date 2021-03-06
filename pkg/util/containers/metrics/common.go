// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package metrics

import (
	"bufio"
	"os"
	"path/filepath"

	"github.com/ninnemana/datadog-agent/pkg/config"
)

// readLines reads contents from a file and splits them by new lines.
func readLines(filename string) ([]string, error) {
	f, err := os.Open(filename)
	if err != nil {
		return []string{""}, err
	}
	defer f.Close()

	var ret []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		ret = append(ret, scanner.Text())
	}
	return ret, scanner.Err()
}

// hostProc returns the location of a host's procfs. This can and will be
// overridden when running inside a container.
func hostProc(combineWith ...string) string {
	parts := append([]string{config.Datadog.GetString("container_proc_root")}, combineWith...)
	return filepath.Join(parts...)
}

// pathExists returns a boolean indicating if the given path exists on the file system.
func pathExists(filename string) bool {
	if _, err := os.Stat(filename); err == nil {
		return true
	}
	return false
}
