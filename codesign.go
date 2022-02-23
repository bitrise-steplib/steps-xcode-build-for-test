package main

import (
	"fmt"

	"github.com/bitrise-io/go-steputils/v2/stepconf"
	"github.com/bitrise-io/go-utils/retry"
	"github.com/bitrise-io/go-utils/v2/command"
	"github.com/bitrise-io/go-utils/v2/log"
	"github.com/bitrise-io/go-xcode/devportalservice"
	"github.com/bitrise-io/go-xcode/v2/autocodesign"
	"github.com/bitrise-io/go-xcode/v2/autocodesign/certdownloader"
	"github.com/bitrise-io/go-xcode/v2/autocodesign/codesignasset"
	"github.com/bitrise-io/go-xcode/v2/autocodesign/devportalclient"
	"github.com/bitrise-io/go-xcode/v2/autocodesign/localcodesignasset"
	"github.com/bitrise-io/go-xcode/v2/autocodesign/projectmanager"
	"github.com/bitrise-io/go-xcode/v2/codesign"
)

type CodesignManagerOpts struct {
	CodeSigningAuthSource     string
	CertificateURLList        string
	CertificatePassphraseList stepconf.Secret
	KeychainPath              string
	KeychainPassword          stepconf.Secret
	BuildURL                  string
	BuildAPIToken             stepconf.Secret
	TeamID                    string
	RegisterTestDevices       bool
	MinDaysProfileValid       int
	VerboseLog                bool

	ProjectPath   string
	Scheme        string
	Configuration string
}

func createCodesignManager(managerOpts CodesignManagerOpts, xcodeMajorVersion int64, logger log.Logger, cmdFactory command.Factory) (codesign.Manager, error) {
	var authType codesign.AuthType
	switch managerOpts.CodeSigningAuthSource {
	case codeSignSourceAppleID:
		authType = codesign.AppleIDAuth
	case codeSignSourceAPIKey:
		authType = codesign.APIKeyAuth
	case codeSignSourceOff:
		return codesign.Manager{}, fmt.Errorf("automatic code signing is disabled")
	}

	codesignInputs := codesign.Input{
		AuthType:                  authType,
		DistributionMethod:        string(autocodesign.Development),
		CertificateURLList:        managerOpts.CertificateURLList,
		CertificatePassphraseList: managerOpts.CertificatePassphraseList,
		KeychainPath:              managerOpts.KeychainPath,
		KeychainPassword:          managerOpts.KeychainPassword,
	}

	codesignConfig, err := codesign.ParseConfig(codesignInputs, cmdFactory)
	if err != nil {
		return codesign.Manager{}, fmt.Errorf("issue with input: %s", err)
	}

	var serviceConnection *devportalservice.AppleDeveloperConnection = nil
	devPortalClientFactory := devportalclient.NewFactory(logger)
	if authType == codesign.APIKeyAuth || authType == codesign.AppleIDAuth {
		if serviceConnection, err = devPortalClientFactory.CreateBitriseConnection(managerOpts.BuildURL, string(managerOpts.BuildAPIToken)); err != nil {
			return codesign.Manager{}, err
		}
	}

	appleAuthCredentials, err := codesign.SelectConnectionCredentials(authType, serviceConnection, logger)
	if err != nil {
		return codesign.Manager{}, err
	}

	opts := codesign.Opts{
		AuthType:                   authType,
		ShouldConsiderXcodeSigning: true,
		TeamID:                     managerOpts.TeamID,
		ExportMethod:               codesignConfig.DistributionMethod,
		XcodeMajorVersion:          int(xcodeMajorVersion),
		RegisterTestDevices:        managerOpts.RegisterTestDevices,
		SignUITests:                true,
		MinDaysProfileValidity:     managerOpts.MinDaysProfileValid,
		IsVerboseLog:               managerOpts.VerboseLog,
	}

	project, err := projectmanager.NewProject(projectmanager.InitParams{
		ProjectOrWorkspacePath: managerOpts.ProjectPath,
		SchemeName:             managerOpts.Scheme,
		ConfigurationName:      managerOpts.Configuration,
	})
	if err != nil {
		return codesign.Manager{}, fmt.Errorf("failed to open project: %s", err)
	}

	return codesign.NewManagerWithProject(
		opts,
		appleAuthCredentials,
		serviceConnection,
		devPortalClientFactory,
		certdownloader.NewDownloader(codesignConfig.CertificatesAndPassphrases, retry.NewHTTPClient().StandardClient()),
		codesignasset.NewWriter(codesignConfig.Keychain),
		localcodesignasset.NewManager(localcodesignasset.NewProvisioningProfileProvider(), localcodesignasset.NewProvisioningProfileConverter()),
		project,
		logger,
	), nil
}
