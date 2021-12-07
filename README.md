# Xcode Build for testing for iOS

[![Step changelog](https://shields.io/github/v/release/bitrise-steplib/steps-xcode-build-for-test?include_prereleases&label=changelog&color=blueviolet)](https://github.com/bitrise-steplib/steps-xcode-build-for-test/releases)

Performs xcodebuild's build-for-testing action

<details>
<summary>Description</summary>

The Step runs Xcode's `xcodebuild` command with the `build-for-testing` option. This builds your app for testing and also creates an `.xctestrun` file.

### Configuring the Step

At a minimum, the Step needs valid values for three inputs:

- **Project (or Workspace) path**: This is the path to the `.xcodeproj` or `.xcworkspace` file. In most cases, leave it on the default value.
- **Scheme name**: The name of your Xcode scheme. By default, the Step will use the scheme that was set when you added the app on Bitrise.
- **Device destination**: The device and platform type to build the tests for. For available values call, `man xcodebuild` and check the Destinations section.
We also recommend checking out our [System reports page](https://github.com/bitrise-io/bitrise.io/tree/master/system_reports) on GitHub: you can check out the available, pre-installed simulators and other tools.

Optionally, you can define the configuration to use in the **Configuration name** input. Normally, the scheme defines the configuration type, such as **debug** or **release**.

The Step can also cache your Swift PM dependencies. To enable caching, make sure the **Enable caching of Swift Package Manager packages** input is set to `swift_packages`.

### Troubleshooting

In the **Debug** option group, you can:

- Add additional flags to the xcodebuild command.
- Enable verbose logging.
- Change the output directory path and the output tool.

### Useful links

- [Running Xcode tests](https://devcenter.bitrise.io/testing/running-xcode-tests/)
- [Building from the Command Line with Xcode](https://developer.apple.com/library/archive/technotes/tn2339/_index.html)

### Related Steps

- [Xcode Test for iOS](https://www.bitrise.io/integrations/steps/xcode-test)
- [Xcode Analyze](https://www.bitrise.io/integrations/steps/xcode-analyze)
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
| `scheme` | Xcode Scheme name.  The input value sets xcodebuild's `-scheme` option. | required | `$BITRISE_SCHEME` |
| `configuration` | Xcode Build Configuration.  If not specified, the default Build Configuration will be used.  The input value sets xcodebuild's `-configuration` option. | required | `Debug` |
| `destination` | Destination specifier describes the device to use as a destination.  The input value sets xcodebuild's `-destination` option. | required | `generic/platform=iOS` |
| `xcconfig_content` | Build settings to override the project's build settings.  Build settings must be separated by newline character (`\n`).  Example:  ``` COMPILER_INDEX_STORE_ENABLE = NO ONLY_ACTIVE_ARCH[config=Debug][sdk=*][arch=*] = YES ```  The input value sets xcodebuild's `-xcconfig` option. |  | `COMPILER_INDEX_STORE_ENABLE = NO` |
| `xcodebuild_options` | Additional options to be added to the executed xcodebuild command. |  |  |
| `log_formatter` | Defines how xcodebuild command's log is formatted.  Available options: - `xcpretty`: The xcodebuild command’s output will be prettified by xcpretty. - `xcodebuild`: Only the last 20 lines of raw xcodebuild output will be visible in the build log.  The raw xcodebuild log will be exported in both cases. | required | `xcpretty` |
| `automatic_code_signing` | This input determines which Bitrise Apple service connection should be used for automatic code signing.  Available values: - `off`: Do not do any auto code signing. - `api-key`: [Bitrise Apple Service connection with API Key](https://devcenter.bitrise.io/getting-started/connecting-to-services/setting-up-connection-to-an-apple-service-with-api-key/). - `apple-id`: [Bitrise Apple Service connection with Apple ID](https://devcenter.bitrise.io/getting-started/connecting-to-services/connecting-to-an-apple-service-with-apple-id/). | required | `off` |
| `register_test_devices` | If this input is set, the Step will register the known test devices on Bitrise from team members with the Apple Developer Portal.  Note that setting this to yes may cause devices to be registered against your limited quantity of test devices in the Apple Developer Portal, which can only be removed once annually during your renewal window. | required | `no` |
| `min_profile_validity` | If this input is set to >0, the managed Provisioning Profile will be renewed if it expires within the configured number of days.  Otherwise the Step renews the managed Provisioning Profile if it is expired. | required | `0` |
| `certificate_url_list` | URL of the code signing certificate to download.  Multiple URLs can be specified, separated by a pipe (`\|`) character.  Local file path can be specified, using the `file://` URL scheme. | required, sensitive | `$BITRISE_CERTIFICATE_URL` |
| `passphrase_list` | Passphrases for the provided code signing certificates.  Specify as many passphrases as many Code signing certificate URL provided, separated by a pipe (`\|`) character. | required, sensitive | `$BITRISE_CERTIFICATE_PASSPHRASE` |
| `keychain_path` | Path to the Keychain where the code signing certificates will be installed. | required | `$HOME/Library/Keychains/login.keychain` |
| `keychain_password` | Password for the provided Keychain. | required, sensitive | `$BITRISE_KEYCHAIN_PASSWORD` |
| `output_dir` | This directory will contain the generated artifacts. | required | `$BITRISE_DEPLOY_DIR` |
| `cache_level` | Defines what cache content should be automatically collected.  Available options: - `none`: Disable collecting cache content. - `swift_packages`: Collect Swift PM packages added to the Xcode project. | required | `swift_packages` |
| `verbose_log` | If this input is set, the Step will print additional logs for debugging. | required | `no` |
</details>

<details>
<summary>Outputs</summary>

| Environment Variable | Description |
| --- | --- |
| `BITRISE_TEST_DIR_PATH` | Path to the built test directory (example: `PROJECT_DERIVED_DATA/Build/Products/Debug-iphoneos`) |
| `BITRISE_XCTESTRUN_FILE_PATH` | Path to the built xctestrun file (example: `PROJECT_DERIVED_DATA/Build/Products/ios-simple-objc_iphoneos12.0-arm64e.xctestrun`) |
| `BITRISE_TEST_BUNDLE_ZIP_PATH` | The built test directory and the built xctestrun file compressed as a single zip |
| `BITRISE_XCODE_RAW_RESULT_TEXT_PATH` | The file path of the raw `xcodebuild build-for-testing` command log. |
</details>

## 🙋 Contributing

We welcome [pull requests](https://github.com/bitrise-steplib/steps-xcode-build-for-test/pulls) and [issues](https://github.com/bitrise-steplib/steps-xcode-build-for-test/issues) against this repository.

For pull requests, work on your changes in a forked repository and use the Bitrise CLI to [run step tests locally](https://devcenter.bitrise.io/bitrise-cli/run-your-first-build/).

Learn more about developing steps:

- [Create your own step](https://devcenter.bitrise.io/contributors/create-your-own-step/)
- [Testing your Step](https://devcenter.bitrise.io/contributors/testing-and-versioning-your-steps/)
