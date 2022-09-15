package opa

import (
	_ "embed"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/open-policy-agent/opa/rego"
	"github.com/pkg/errors"
)

const moduleFileExt = ".rego"

func filterModule(path string) bool {
	return strings.HasSuffix(path, moduleFileExt)
}

type Module struct {
	Filename string
	Contents string
}

func (m Module) WriteFile(parent string, perm os.FileMode) (n int, err error) {
	dir := filepath.Clean(filepath.Join(parent, filepath.Dir(m.Filename)))
	if err = os.MkdirAll(dir, perm); err != nil {
		return 0, err
	}

	path := filepath.Clean(filepath.Join(dir, filepath.Base(m.Filename)))
	file, err := os.Create(path)
	if err != nil {
		return 0, err
	}
	defer func() {
		if cerr := file.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	return file.WriteString(m.Contents)
}

func CollectModules(modules ...[]Module) (collected []Module) {
	collected = make([]Module, 0, len(modules))
	for _, m := range modules {
		collected = append(collected, m...)
	}
	return collected
}

func Modules(modules ...Module) func(r *rego.Rego) {
	return func(r *rego.Rego) {
		for _, module := range modules {
			rego.Module(module.Filename, module.Contents)(r)
		}
	}
}

func readFiles(f fs.FS, filter func(path string) bool) ([]Module, error) {
	modules := []Module{}
	err := fs.WalkDir(f, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if filter == nil || filter(path) {
			b, err := fs.ReadFile(f, path)
			if err != nil {
				return errors.Wrapf(nil, "couldn't read file %s", path)
			}
			modules = append(modules, Module{
				Filename: path,
				Contents: string(b),
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return modules, nil
}

func FSModules(fsys fs.FS, filePrefix string) ([]Module, error) {
	modules, err := readFiles(fsys, filterModule)
	if err != nil {
		return nil, errors.Wrap(err, "couldn't list rego modules")
	}
	qualifiedModules := make([]Module, len(modules))
	for i, module := range modules {
		qualifiedModules[i] = Module{
			Filename: filePrefix + module.Filename,
			Contents: module.Contents,
		}
	}
	return qualifiedModules, nil
}

//go:embed routing.rego
var routingModule string

//go:embed errors.rego
var errorsModule string

func RegoModules() []Module {
	const packagePath = "github.com/sargassum-world/godest/opa"
	return []Module{
		{Filename: packagePath + "/routing.rego", Contents: routingModule},
		{Filename: packagePath + "/errors.rego", Contents: errorsModule},
	}
}
