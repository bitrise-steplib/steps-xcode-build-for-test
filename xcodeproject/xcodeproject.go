package xcodeproject

import (
	"github.com/bitrise-io/go-xcode/xcodeproject/schemeint"
	"github.com/bitrise-io/go-xcode/xcodeproject/xcscheme"
)

type XcodeProject interface {
	Scheme(pth string, name string) (*xcscheme.Scheme, error)
}

type xcodeProject struct {
}

func NewXcodeProject() XcodeProject {
	return xcodeProject{}
}

func (p xcodeProject) Scheme(projectPath string, schemeName string) (*xcscheme.Scheme, error) {
	scheme, _, err := schemeint.Scheme(projectPath, schemeName)
	return scheme, err
}
