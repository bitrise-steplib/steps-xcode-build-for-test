package xcodeproject

import (
	"github.com/bitrise-io/go-xcode/v2/xcodeproject"
	"github.com/bitrise-io/go-xcode/v2/xcodeproject/xcscheme"
)

type XcodeProject interface {
	Scheme(pth string, name string) (*xcscheme.Scheme, error)
}

type xcodeProject struct {
	factory xcodeproject.Factory
}

func NewXcodeProject(factory xcodeproject.Factory) XcodeProject {
	return xcodeProject{factory: factory}
}

func (p xcodeProject) Scheme(projectPath string, schemeName string) (*xcscheme.Scheme, error) {
	scheme, err := p.factory.Scheme(projectPath, schemeName)
	if err != nil {
		return nil, err
	}
	return &scheme, nil
}
