<p align="center">
    <img src="./logo.svg" width="300"/>
<p>

# Simple Observability Agent (simob)

> Lightweight agent to collect system metrics and logs — with zero dependencies and one-command setup.

**Simple Observability Agent (simob)** is a lightweight agent designed to collect monitoring
data from servers and forward it to [SimpleObservability.com](https://simpleobservability.com)
endpoints.

The agent is built to be plug-and-play, requiring minimal server-side configuration. It has no
runtime dependencies and a one command install process.

## Table of Contents

- [Installation](#installation)
- [Usage](#usage)
- [Documentation](#documentation)

## Installation

### Prebuilt binaries
You can install the agent with a single command:
```bash
$ curl -fsSL https://simpleobservability.com/install.sh | sudo bash -s -- <SERVER_KEY> [optional flags]
```
> [!IMPORTANT]
> Replace `<SERVER_KEY>` with the one from your SimpleObservability.com account.

This will:
 - Create a dedicated user and group
 - Download the latest agent binary
 - Set up a systemd service
 - Initialize the agent with the provided server key

> [!NOTE]
> The install script is fully documented with verbose comments and is designed to be easy to read, understand, and audit.


### Optional flags

The install script supports some optional flags that control how the agent is installed and what permissions it has.

#### `--no-journal-access`
By default, the install script grants the `simob-agent` user read access to system logs by adding it to the `systemd-journal` group.
If you don’t want the agent to read system logs you can disable this permission during installation:

### From source
You can also build the agent binaries from source. During installation, set the environment variable `BINARY_PATH` to point to your built binary:

```bash
$ sudo BINARY_PATH=<PATH TO BINARY> bash install.sh <SERVER_KEY>
```

## Usage
Once installed, the agent binary (`simob`) is available in your system's PATH.

You can interact with it using the following commands:

- `simob init`
  Initialize the agent by generating a default configuration Discovers available metrics and log sources.
  Also gather basic info about the server (hostname, os, arch and agent version).
  > **Optional** if you installed the agent using the install script, the script already runs this for you.

- `simob start`
  Starts the collection service manually (used internally by systemd).
  You generally don’t need to run this unless debugging.

- `simob update`
  Checks for and installs the latest version of the agent binary.

- `simob version`
  Prints the currently installed agent version.

- `simob config`:
  Outputs the current resolved configuration. Helpful for debugging and validating setup.

## Documentation
For more detailed information, including advanced configuration and troubleshooting, please visit our [official documentation](https://simpleobservability.com/docs).