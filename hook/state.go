package hook

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/opencontainers/runtime-spec/specs-go"
)

func State() (specs.State, error) {
	stdinBytes, err := ioutil.ReadAll(os.Stdin)
	if err != nil {
		return specs.State{}, err
	}
	var state specs.State
	err = json.Unmarshal(stdinBytes, &state)
	if err != nil {
		return specs.State{}, err
	}
	return state, nil
}

func Config(state specs.State) (specs.Spec, error) {
	configBytes, err := ioutil.ReadFile(filepath.Join(state.Bundle, "config.json"))
	if err != nil {
		return specs.Spec{}, err
	}
	var spec specs.Spec
	if err := json.Unmarshal(configBytes, &spec); err != nil {
		return specs.Spec{}, err
	}
	return spec, nil
}

func Root(bundlePath string, spec specs.Spec) (string, error) {
	if spec.Root == nil || len(spec.Root.Path) == 0 {
		return "", errors.New("undefined root path")
	}
	path := spec.Root.Path
	if path[0] != '/' {
		path = filepath.Join(bundlePath, path)
	}
	return path, nil
}
