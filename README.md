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
- [Available Inputs](#available-inputs)

## Installation

You can install the agent with a single command:
```bash
$ curl -fL https://simpleobservability.com/install.sh | sudo bash -s -- <SERVER_KEY>
```

Replace <SERVER_KEY> with the one from your SimpleObservability.com account.

This will:
 - Create a dedicated user and group
 - Download the latest agent binary
 - Set up a systemd service
 - Initialize the agent with the provided server key

The install script is fully documented with verbose comments and is designed to be easy to read,
understand, and audit.

## Usage
Once installed, the agent binary (`simob`) is available in your system's PATH.

You can interact with it using the following commands:

### Commands
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

## Available Inputs

### Metrics

| Input           | Details                               | Status  |
|-----------------|---------------------------------------|---------|
| CPU             | Per-core and aggregate usage          |   ✅    |
| Memory          | RAM usage                             |   ✅    |
| Network         | Interface stats (bytes, packets)      |   ✅    |
| Disk usage      | Used/free space per mount             |   ✅    |
| Disk IO         | Read/write operations per device      |         |
| Kernel          | Kernel metrics                        |         |
| Processes       | Count and stats                       |         |
| Swap            | Swap usage stats                      |         |
| Docker          | Container metrics                     |         |
| fail2ban        | Status and actions                    |         |
| HDD temperature | SMART disk temps                      |         |
| Internet speed  | Bandwidth tests                       |         |
| Sensors         | Hardware sensors                      |         |
| Temperature     | System temperature sensors            |         |

### Logs

| Input          | Details                          | Status  |
|----------------|----------------------------------|---------|
| NGINX          | Access and error logs            | ✅      |
| Apache         | Access and error logs            |         |
| Authentication | `/var/log/auth.log` and variants |         |
| fail2ban       | `/var/log/fail2ban.log`          |         |
