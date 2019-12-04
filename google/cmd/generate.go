package main

import (
	"bytes"
	"io"
	"os"
	"os/exec"

	"github.com/pkg/errors"
)

var functions = []Function{
	Function{Resource: "Instance", Zone: true},
	Function{Resource: "Firewall", Zone: false},
	Function{Resource: "Network", Zone: false},
	Function{Resource: "InstanceGroup", Zone: true},
	Function{Resource: "BackendService", Zone: false},
	Function{Resource: "HealthCheck", Zone: false},
}

func main() {
	f, err := os.OpenFile("./reader_generated.go", os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	if err := generate(f, functions); err != nil {
		panic(err)
	}
}

func generate(opt io.Writer, fns []Function) error {
	var fnBuff = bytes.Buffer{}

	if err := pkgTmpl.Execute(&fnBuff, nil); err != nil {
		return errors.Wrap(err, "unable to execute package template")
	}

	for _, function := range functions {
		if err := function.Execute(&fnBuff); err != nil {
			return errors.Wrapf(err, "unable to execute function template for: %s", function.Resource)
		}
	}

	// format
	cmd := exec.Command("goimports")
	cmd.Stdin = &fnBuff
	cmd.Stdout = opt
	if err := cmd.Run(); err != nil {
		return errors.Wrap(err, "unable to run goimports command")
	}
	return nil
}