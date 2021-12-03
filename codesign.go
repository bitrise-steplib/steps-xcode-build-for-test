package main

import (
	"fmt"

	"github.com/bitrise-io/go-utils/command"
	"github.com/bitrise-io/go-utils/log"
	"github.com/bitrise-io/go-utils/retry"
	"github.com/bitrise-io/go-xcode/autocodesign"
	"github.com/bitrise-io/go-xcode/autocodesign/certdownloader"
	"github.com/bitrise-io/go-xcode/autocodesign/codesignasset"
	"github.com/bitrise-io/go-xcode/autocodesign/devportalclient"
	"github.com/bitrise-io/go-xcode/autocodesign/localcodesignasset"
	"github.com/bitrise-io/go-xcode/autocodesign/projectmanager"
	"github.com/bitrise-io/go-xcode/codesign"
	"github.com/bitrise-io/go-xcode/devportalservice"
)

func createCodesignManager(config Config, xcodeMajorVersion int64, logger log.Logger, cmdFactory command.Factory) (codesign.Manager, error) {
	var authType codesign.AuthType
	switch config.CodeSigningAuthSource {
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
		CertificateURLList:        config.CertificateURLList,
		CertificatePassphraseList: config.CertificatePassphraseList,
		KeychainPath:              config.KeychainPath,
		KeychainPassword:          config.KeychainPassword,
	}

	codesignConfig, err := codesign.ParseConfig(codesignInputs, cmdFactory)
	if err != nil {
		return codesign.Manager{}, fmt.Errorf("issue with input: %s", err)
	}

	var serviceConnection *devportalservice.AppleDeveloperConnection = nil
	devPortalClientFactory := devportalclient.NewFactory(logger)
	if authType == codesign.APIKeyAuth || authType == codesign.AppleIDAuth {
		if serviceConnection, err = devPortalClientFactory.CreateBitriseConnection(config.BuildURL, string(config.BuildAPIToken)); err != nil {
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
		TeamID:                     "",
		ExportMethod:               codesignConfig.DistributionMethod,
		XcodeMajorVersion:          int(xcodeMajorVersion),
		RegisterTestDevices:        config.RegisterTestDevices,
		SignUITests:                true,
		MinDaysProfileValidity:     config.MinDaysProfileValid,
		IsVerboseLog:               config.VerboseLog,
	}

	return codesign.NewManager(
		opts,
		appleAuthCredentials,
		serviceConnection,
		devPortalClientFactory,
		certdownloader.NewDownloader(codesignConfig.CertificatesAndPassphrases, retry.NewHTTPClient().StandardClient()),
		codesignasset.NewWriter(codesignConfig.Keychain),
		localcodesignasset.NewManager(localcodesignasset.NewProvisioningProfileProvider(), localcodesignasset.NewProvisioningProfileConverter()),
		projectmanager.NewFactory(projectmanager.InitParams{
			ProjectOrWorkspacePath: config.ProjectPath,
			SchemeName:             config.Scheme,
			ConfigurationName:      config.Configuration,
		}),
		logger,
	), nil
}
