<div align="center">
  <picture>
    <source media="(prefers-color-scheme: light)" srcset="./logo-primary.svg">
    <img alt="Simple Observability logo" src="./logo-white.svg" width="180">
  </picture>
</div>

<div align="center">

**Simple Observability Agent** — `simob`

Simple, lightweight, zero-config monitoring for your servers.

<h3>
  <a href="https://simpleobservability.com">Homepage</a> |
  <a href="https://simpleobservability.com/docs">Docs</a> |
  <a href="https://app.simpleobservability.com">App</a>
</h3>

[![Build](https://github.com/simple-observability/simob-agent/actions/workflows/build.yml/badge.svg)](https://github.com/simple-observability/simob-agent/actions/workflows/build.yml)
[![Release](https://img.shields.io/github/release/simple-observability/simob-agent?color=blue)](https://github.com/simple-observability/simob-agent/actions/workflows/build.yml)
[![License](https://img.shields.io/github/license/simple-observability/simob-agent?color=yellow)](https://github.com/simple-observability/simob-agent/releases/latest)

</div>

---

Simple Observability Agent **(simob)** is a lightweight agent designed to
collect monitoring data from servers and forward it to
[SimpleObservability.com](https://simpleobservability.com) endpoints.

The agent is built to be plug-and-play, requiring no server-side configuration.
It's distributed as a standalone binary, with no runtime dependencies and a one
command install process.

## About Simple Observability

Simple Observability is a server monitoring platform built on a simple idea that
monitoring should be as easy as installing one probe on your server. The monitoring
platform takes care of the rest.

### Getting Started
  1. Install the agent: Install the open-source agent with a single command.
  2. Configure from the UI: Manage everything from the web interface. No config files needed.
  3. Get instant insights: Access ready-to-use reports and alerts.

### Key Features
  * Unified monitoring: Metrics and logs from all your servers in one place.
  * Built-in alerts: Predefined and customizable alert rules.
  * Full web configuration: Everything managed from the UI.
  * One-command install: Lightweight agent, no dependencies.
  * Ready-to-use reports: Instant visibility, no setup required.
  * Mobile app: Access your dashboards anywhere.

## Installation

### Prebuilt binaries
You can install the agent with a single command:
```bash
$ curl -fsSL https://simpleobservability.com/install.sh | sudo bash -s -- <SERVER_KEY>
```

Replace <SERVER_KEY> with the one from your SimpleObservability.com account.

This will:
 - Create a dedicated user and group
 - Download the latest agent binary
 - Set up a systemd service
 - Initialize the agent with the provided server key

The install script is fully documented with verbose comments and is designed to be easy
to read, understand, and audit.

### From source
You can also build the agent binaries from source. During installation, set the environment
variable `BINARY_PATH` to point to your built binary:

```bash
$ sudo BINARY_PATH=<PATH TO BINARY> ./install.sh <SERVER_KEY>
```

## Usage
Once installed, the agent binary (`simob`) is available in your system's PATH.

You can interact with it using the following commands:

| Command             | Description                                                                                                                 |
|---------------------|-----------------------------------------------------------------------------------------------------------------------------|
| **`simob init`**    | Initializes the agent by discovering available metrics and log sources, and collecting basic system info.                   |
| **`simob start`**   | Starts the collection service manually. This command is used internally by `systemd`. You generally don’t need to run this. |
| **`simob status`**  | Checks if the agent is currently running.                                                                                   |
| **`simob update`**  | Checks for and installs the latest version of the agent binary.                                                             |
| **`simob version`** | Prints the currently installed agent version.                                                                               |
| **`simob config`**  | Outputs the current resolved configuration.                                                                                 |

## Collected Data

The `simob` agent automatically discovers and collects metrics and logs from a wide range
of sources

### Metrics

| Category    | Description                                                             |
|-------------|-------------------------------------------------------------------------|
| **CPU**     | Tracks per-core and total CPU utilization, load, and idle time.         |
| **Memory**  | Monitors total, used, and available system memory.                      |
| **Disk**    | Reports disk usage, free space, and read/write activity.                |
| **Network** | Measures incoming/outgoing traffic, packet counts, and errors.          |
| **NGINX**   | Collects active connections and request rates from the status endpoint. |

### Log Sources

| Source      | Description           |
|-------------|-----------------------|
| **Apache**  | Collects access logs. |
| **NGINX**   | Collects NGINX logs.  |

## Documentation
For more detailed information, including advanced configuration and troubleshooting,
please visit our [official documentation](https://simpleobservability.com/docs).

## FAQ

### What are the requirements to run the agent?
The Simple Observability agent is a **single, static binary** — no Docker, no dependencies,
no runtime.
It runs on most Linux distributions out of the box.
If you prefer, you can **build it from source** directly from this repository.

### Do I need to open any ports?
No inbound ports are required.
The agent uses a **push model** over **HTTPS (port 443)** to `*.simpleobservability.com`.

### What if I need a custom data source?
You can easily extend collection logic for new logs or metrics.
If your source isn’t supported yet, just **open a GitHub issue** describing your use
case — we’ll review it and help you get it integrated.