# Xcode Build for testing for iOS

[![Step changelog](https://shields.io/github/v/release/bitrise-steplib/steps-xcode-build-for-test?include_prereleases&label=changelog&color=blueviolet)](https://github.com/bitrise-steplib/steps-xcode-build-for-test/releases)

Performs xcodebuild's build-for-testing action

<details>
<summary>Description</summary>

The Step runs Xcode's `xcodebuild` command with the `build-for-testing` option. This builds your app and associated tests so that you can, for example, upload it to a third-party testing service to run your tests on a real device.
The Step also creates an `.xctestrun` file.
To be able to run your tests on a real device it needs code signing. The **Automatic code signing method** Step input allows you to log you into your Apple Developer account based on the [Apple service connection you provide on Bitrise](https://devcenter.bitrise.io/en/accounts/connecting-to-services/apple-services-connection.html) and download any provisioning profiles needed for your project based on the **Distribution method**.
Please note that the **Automatic code signing method** input is by default set to `off`, so if you need code signing, select either the `api-key` or the `apple-id`option of the input.

### Configuring the Step
Before you start:
- Make sure you have connected your [Apple Service account to Bitrise](https://devcenter.bitrise.io/en/accounts/connecting-to-services/apple-services-connection.html).
Alternatively, you can upload certificates and profiles to Bitrise manually, then use the **Certificate and Profile Installer** Step before this Step.
- Make sure certificates are uploaded to Bitrise's **Code Signing** tab. The right provisioning profiles are automatically downloaded from Apple as part of the automatic code signing process.

To configure the Step:
1. **Project (or Workspace) path**: This is the path where the `.xcodeproj` or `.xcworkspace` files are localed.
2. **Scheme**: Add the scheme name you wish to build for testing.
3. **Build Configuration**: If not specified, the default Build Configuration will be used. The input value sets xcodebuild's `-configuration` option.
4. **Device destination specifier**: Destination specifier describes the device to use as a destination. The input value sets xcodebuild's `-destination` option.

Under **xcodebuild configuration**
5. **Build settings (xcconfig)**:  Build settings to override the project's build settings. Can be the contents, file path or empty.
6. **Additional options for the xcodebuild command**:  Additional options to be added to the executed xcodebuild command.

Under **Xcode build log formatting**:
1. **Log formatter**: Defines how `xcodebuild` command's log is formatted. Available options: `xcpretty`: The xcodebuild command's output will be prettified by xcpretty. `xcodebuild`: Only the last 20 lines of raw xcodebuild output will be visible in the build log. The raw xcodebuild log is exported in both cases.

Under **Automatic code signing**:
1. **Automatic code signing method**: Select the Apple service connection you want to use for code signing. Available options: `off` if you don't do automatic code signing, `api-key` [if you use API key authorization](https://devcenter.bitrise.io/en/accounts/connecting-to-services/connecting-to-an-apple-service-with-api-key.html), and `apple-id` [if you use Apple ID authorization](https://devcenter.bitrise.io/en/accounts/connecting-to-services/connecting-to-an-apple-service-with-apple-id.html).
2. **Register test devices on the Apple Developer Portal**: If this input is set, the Step will register the known test devices on Bitrise from team members with the Apple Developer Portal. Note that setting this to `yes` may cause devices to be registered against your limited quantity of test devices in the Apple Developer Portal, which can only be removed once annually during your renewal window.
3. **The minimum days the Provisioning Profile should be valid**: If this input is set to >0, the managed Provisioning Profile will be renewed if it expires within the configured number of days. Otherwise the Step renews the managed Provisioning Profile if it is expired.
4. The **Code signing certificate URL**, the **Code signing certificate passphrase**, the **Keychain path**, and the **Keychain password** inputs are automatically populated if certificates are uploaded to Bitrise's **Code Signing** tab. If you store your files in a private repo, you can manually edit these fields.

If you want to set the Apple service connection credentials on the step-level (instead of using the one configured in the App Settings), use the Step inputs in the **App Store Connect connection override** category. Note that this only works if **Automatic code signing method** is set to `api-key`.

Under **Step Output configuration**:
1. **Output directory path**: This directory contains the generated artifacts.

Under **Caching**:
1. **Enable collecting cache content**: Defines what cache content should be automatically collected. Available options are:
  - `none`: Disable collecting cache content
  - `swift_packages`: Collect Swift PM packages added to the Xcode project

Under Debugging:
1. **Verbose logging***: You can set this input to `yes` to produce more informative logs.
</details>

## 🧩 Get started

Add this step directly to your workflow in the [Bitrise Workflow Editor](https://devcenter.bitrise.io/steps-and-workflows/steps-and-workflows-index/).

You can also run this step directly with [Bitrise CLI](https://github.com/bitrise-io/bitrise).

## ⚙️ Configuration

<details>
<summary>Inputs</summary>

| Key | Description | Flags | Default |
| --- | --- | --- | --- |
| `project_path` | Xcode Project (`.xcodeproj`) or Workspace (`.xcworkspace`) path.  The input value sets xcodebuild's `-project` or `-workspace` option. | required | `$BITRISE_PROJECT_PATH` |
| `test_plan` | Build tests for a specific Test Plan associated with the Scheme.  Leave this input empty to build all the Test Plans or Test Targets associated with the Scheme.  The input value sets xcodebuild's `-testPlan` option. |  |  |
| `scheme` | Xcode Scheme name.  The input value sets xcodebuild's `-scheme` option. | required | `$BITRISE_SCHEME` |
| `configuration` | Xcode Build Configuration.  If not specified, the default Build Configuration will be used.  The input value sets xcodebuild's `-configuration` option. | required | `Debug` |
| `destination` | Destination specifier describes the device to use as a destination.  Recommended values: - `generic/platform=iOS` to build tests for physical devices - `generic/platform=iOS Simulator` to build tests for Simulators  The input value sets xcodebuild's `-destination` option. | required | `generic/platform=iOS` |
| `skip_testing` | The step will skip the listed tests during building your app and tests.  Based on this input, the specified Test Plan will be modified to exclude the listed tests,  therefore, specifying the Test Plan is mandatory.  You can enter multiple test identifiers separated by a newline. The input field supports the following test identifier formats: - Test target and test class name: `MyAppTests/MyAppTests` - Test target, class and function name: `MyAppTests/MyAppTests/testExample`  The input value can be a filepath as well which contains the list of tests separated by a newline character. |  |  |
| `xcconfig_content` | Build settings to override the project's build settings, using xcodebuild's `-xcconfig` option.  You can't define `-xcconfig` option in `Additional options for the xcodebuild command` if this input is set.  If empty, no setting is changed. When set it can be either: 1.  Existing `.xcconfig` file path.      Example:      `./ios-sample/ios-sample/Configurations/Dev.xcconfig`  2.  The contents of a newly created temporary `.xcconfig` file. (This is the default.)      Build settings must be separated by newline character (`\n`).      Example:     ```     COMPILER_INDEX_STORE_ENABLE = NO     ONLY_ACTIVE_ARCH[config=Debug][sdk=*][arch=*] = YES     ``` |  | `COMPILER_INDEX_STORE_ENABLE = NO` |
| `xcodebuild_options` | Additional options to be added to the executed xcodebuild command.  Prefer using `Build settings (xcconfig)` input for specifying `-xcconfig` option. You can't use both. |  |  |
| `log_formatter` | Defines how xcodebuild command's log is formatted.  Available options: - `xcpretty`: The xcodebuild command’s output will be prettified by xcpretty. - `xcodebuild`: Only the last 20 lines of raw xcodebuild output will be visible in the build log.  The raw xcodebuild log will be exported in both cases. | required | `xcpretty` |
| `automatic_code_signing` | This input determines which Bitrise Apple service connection should be used for automatic code signing.  Available values: - `off`: Do not do any auto code signing. - `api-key`: [Bitrise Apple Service connection with API Key](https://devcenter.bitrise.io/getting-started/connecting-to-services/setting-up-connection-to-an-apple-service-with-api-key/). - `apple-id`: [Bitrise Apple Service connection with Apple ID](https://devcenter.bitrise.io/getting-started/connecting-to-services/connecting-to-an-apple-service-with-apple-id/). | required | `off` |
| `register_test_devices` | If this input is set, the Step will register the known test devices on Bitrise from team members with the Apple Developer Portal.  Note that setting this to yes may cause devices to be registered against your limited quantity of test devices in the Apple Developer Portal, which can only be removed once annually during your renewal window. | required | `no` |
| `test_device_list_path` | If this input is set, the Step will register the listed devices from this file with the Apple Developer Portal.  The format of the file is a comma separated list of the identifiers. For example: `00000000–0000000000000001,00000000–0000000000000002,00000000–0000000000000003`  And in the above example the registered devices appear with the name of `Device 1`, `Device 2` and `Device 3` in the Apple Developer Portal.  Note that setting this will have a higher priority than the Bitrise provided devices list. |  |  |
| `min_profile_validity` | If this input is set to >0, the managed Provisioning Profile will be renewed if it expires within the configured number of days.  Otherwise the Step renews the managed Provisioning Profile if it is expired. | required | `0` |
| `apple_team_id` | The Apple Developer Portal team to use for downloading code signing assets.  Defining this is only required when Automatic Code Signing is set to `apple-id` and the connected account belongs to multiple teams. |  |  |
| `certificate_url_list` | URL of the code signing certificate to download.  Multiple URLs can be specified, separated by a pipe (`\|`) character.  Local file path can be specified, using the `file://` URL scheme. | required, sensitive | `$BITRISE_CERTIFICATE_URL` |
| `passphrase_list` | Passphrases for the provided code signing certificates.  Specify as many passphrases as many Code signing certificate URL provided, separated by a pipe (`\|`) character.  Certificates without a passphrase: for using a single certificate, leave this step input empty. For multiple certificates, use the separator as if there was a passphrase (examples: `pass\|`, `\|pass\|`, `\|`) | sensitive | `$BITRISE_CERTIFICATE_PASSPHRASE` |
| `keychain_path` | Path to the Keychain where the code signing certificates will be installed. | required | `$HOME/Library/Keychains/login.keychain` |
| `keychain_password` | Password for the provided Keychain. | required, sensitive | `$BITRISE_KEYCHAIN_PASSWORD` |
| `fallback_provisioning_profile_url_list` | If set, provided provisioning profiles will be used on Automatic code signing error.  URL of the provisioning profile to download. Multiple URLs can be specified, separated by a newline or pipe (`\|`) character.  You can specify a local path as well, using the `file://` scheme. For example: `file://./BuildAnything.mobileprovision`.  Can also provide a local directory that contains files with `.mobileprovision` extension. For example: `./profilesDirectory/`  | sensitive |  |
| `api_key_path` | Local path or remote URL to the private key (p8 file) for App Store Connect API. This overrides the Bitrise-managed API connection, only set this input if you want to control the API connection on a step-level. Most of the time it's easier to set up the connection on the App Settings page on Bitrise. The input value can be a file path (eg. `$TMPDIR/private_key.p8`) or an HTTPS URL. This input only takes effect if the other two connection override inputs are set too (`api_key_id`, `api_key_issuer_id`). |  |  |
| `api_key_id` | Private key ID used for App Store Connect authentication. This overrides the Bitrise-managed API connection, only set this input if you want to control the API connection on a step-level. Most of the time it's easier to set up the connection on the App Settings page on Bitrise. This input only takes effect if the other two connection override inputs are set too (`api_key_path`, `api_key_issuer_id`). |  |  |
| `api_key_issuer_id` | Private key issuer ID used for App Store Connect authentication. This overrides the Bitrise-managed API connection, only set this input if you want to control the API connection on a step-level. Most of the time it's easier to set up the connection on the App Settings page on Bitrise. This input only takes effect if the other two connection override inputs are set too (`api_key_path`, `api_key_id`). |  |  |
| `api_key_enterprise_account` | Indicates if the account is an enterprise type. This overrides the Bitrise-managed API connection, only set this input if you know you have an enterprise account. | required | `no` |
| `compression_level` | ZIP compression level to control speed / archive size.  Valid values are between 0 and 9. Defaults to 6. - 0: no compression - 1: fastest compression - 9: best compression |  | `6` |
| `output_dir` | This directory will contain the generated artifacts. | required | `$BITRISE_DEPLOY_DIR` |
| `cache_level` | Defines what cache content should be automatically collected.  Available options: - `none`: Disable collecting cache content. - `swift_packages`: Collect Swift PM packages added to the Xcode project. | required | `swift_packages` |
| `verbose_log` | If this input is set, the Step will print additional logs for debugging. | required | `no` |
</details>

<details>
<summary>Outputs</summary>

| Environment Variable | Description |
| --- | --- |
| `BITRISE_TEST_BUNDLE_PATH` | Directory of the built targets' binaries and built associated tests. |
| `BITRISE_TEST_BUNDLE_ZIP_PATH` | Zipped directory of the built targets' binaries and built associated tests. |
| `BITRISE_XCTESTRUN_FILE_PATH` | File path of the built xctestrun file (example: `$SYMROOT/ios-simple-objc_iphoneos12.0-arm64e.xctestrun`).  If `Test Plan` Step Input is set BITRISE_XCTESTRUN_FILE_PATH points to the provided Test Plan's xctestrun file. Otherwise points to the scheme's default Test Plan's xctestrun file (or to the first xctestrun without default Test Plan). |
| `BITRISE_XCODE_RAW_RESULT_TEXT_PATH` | File path of the raw `xcodebuild build-for-testing` command log. |
</details>

## 🙋 Contributing

We welcome [pull requests](https://github.com/bitrise-steplib/steps-xcode-build-for-test/pulls) and [issues](https://github.com/bitrise-steplib/steps-xcode-build-for-test/issues) against this repository.

For pull requests, work on your changes in a forked repository and use the Bitrise CLI to [run step tests locally](https://devcenter.bitrise.io/bitrise-cli/run-your-first-build/).

Learn more about developing steps:

- [Create your own step](https://devcenter.bitrise.io/contributors/create-your-own-step/)
- [Testing your Step](https://devcenter.bitrise.io/contributors/testing-and-versioning-your-steps/)
