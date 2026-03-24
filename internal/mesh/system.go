package mesh

import (
	"encoding/json"
	"os"
	"strings"
)

func readMachineID() (string, error) {
	data, err := os.ReadFile("/etc/machine-id")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func readInstalledSuites(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return []string{}, err
	}

	type stateFile struct {
		InstalledSuites []string `json:"installed_suites"`
		Suites          []string `json:"suites"`
	}

	var state stateFile
	if err := jsonUnmarshal(data, &state); err != nil {
		return []string{}, err
	}
	if len(state.InstalledSuites) > 0 {
		return state.InstalledSuites, nil
	}
	return state.Suites, nil
}

func jsonUnmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}
