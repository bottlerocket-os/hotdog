package main

import (
	"encoding/json"
	"errors"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/bottlerocket/hotdog"
	"github.com/opencontainers/runtime-spec/specs-go"
)

func main() {
	if err := _main(); err != nil {
		panic(err)
	}
}

func _main() error {
	configBytes, err := ioutil.ReadFile("config.json")
	if err != nil {
		return err
	}
	var spec specs.Spec
	if err := json.Unmarshal(configBytes, &spec); err != nil {
		return err
	}
	if spec.Root == nil || len(spec.Root.Path) == 0 {
		return errors.New("undefined root path")
	}
	dest := filepath.Join(spec.Root.Path, hotdog.HotdogContainerDir)
	if stat, err := os.Stat(filepath.Join(spec.Root.Path, hotdog.HotdogContainerDir)); err != nil {
		if _, ok := err.(*os.PathError); !ok {
			// cannot hotpatch
			return nil
		}
		if err := os.Mkdir(filepath.Join(spec.Root.Path, hotdog.HotdogContainerDir), 0755); err != nil {
			return err
		}
	} else if !stat.IsDir() {
		// cannot hotpatch
		return nil
	}
	if err := cp(filepath.Join(hotdog.HotdogDirectory, hotdog.HotdogJDK8Patch), filepath.Join(dest, hotdog.HotdogJDK8Patch)); err != nil {
		return err
	}
	if err := cp(filepath.Join(hotdog.HotdogDirectory, hotdog.HotdogJDK11Patch), filepath.Join(dest, hotdog.HotdogJDK11Patch)); err != nil {
		return err
	}
	if err := cp(filepath.Join(hotdog.HotdogDirectory, hotdog.HotdogJDK17Patch), filepath.Join(dest, hotdog.HotdogJDK17Patch)); err != nil {
		return err
	}
	if err := cp(filepath.Join(hotdog.HotdogDirectory, "hotdog-hotpatch"), filepath.Join(dest, "hotdog-hotpatch")); err != nil {
		return err
	}
	return nil
}

func cp(in, out string) error {
	inReader, err := os.OpenFile(in, os.O_RDONLY, 0)
	if err != nil {
		return err
	}
	outWriter, err := os.OpenFile(out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0755)
	if err != nil {
		return err
	}
	_, err = io.Copy(outWriter, inReader)
	return err
}
