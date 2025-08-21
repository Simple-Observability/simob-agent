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
- [Data collected](#data-collected)

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

The install script is fully documented with verbose comments and is designed to be easy to read,
understand, and audit.

### From source

You can also build the agent binaries from source. During installation, set the environment variable `BINARY_PATH` to point to your built binary:

```bash
$ sudo BINARY_PATH=<PATH TO BINARY> ./install.sh <SERVER_KEY>
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

## Data collected

### Host info
The agent collects basic information about the host machine to provide context for metrics and logs:
- Hostname
- Operating system
- Platform: Name of the OS distribution (e.g., Ubuntu, CentOS)
- Platform family: High-level OS family (e.g., Debian, RHEL)
- Platform version: Version of the distribution (e.g., 22.04)
- Kernel version: Version of the system kernel
- CPU architecture
- Agent version: Version of the simob agent running on the host.

### Metrics

#### Heartbeat
Periodic signal to indicate the agent is running.

#### CPU
Collects per-core and total (aggregated) CPU usage ratios

| Metric                | Description                          |
|-----------------------|--------------------------------------|
| `cpu_user_ratio`      | Time spent in user mode              |
| `cpu_system_ratio`    | Time spent in system/kernel mode     |
| `cpu_idle_ratio`      | Idle time                            |
| `cpu_nice_ratio`      | Time spent on low-priority processes |
| `cpu_iowait_ratio`    | Time waiting for I/O operations      |
| `cpu_irq_ratio`       | Time servicing hardware interrupts   |
| `cpu_softirq_ratio`   | Time servicing software interrupts   |
| `cpu_steal_ratio`     | Time stolen by hypervisor            |
| `cpu_guest_ratio`     | Time running a guest OS              |
| `cpu_guestNice_ratio` | Time running a low-priority guest OS |

**Tags:**
- `cpu`: `total` (aggregated all cores) or `cpu{n}` (per core number)

#### Disk
Monitors disk usage and inode statistics:

| Metric                    | Description       |
|---------------------------|-------------------|
| `disk_total_bytes`        | Total disk space  |
| `disk_free_bytes`         | Free disk space   |
| `disk_used_bytes`         | Used disk space   |
| `disk_used_ratio`         | Used space ratio  |
| `disk_inodes_total_total` | Total inodes      |
| `disk_inodes_free_total`  | Free inodes       |
| `disk_inodes_used_total`  | Used inodes       |
| `disk_inodes_used_ratio`  | Used inodes ratio |

**Tags:**
- `device`: Disk device name (e.g., `/dev/sda1`)
- `mountpoint`: The path where the disk is mounted  (e.g., `/`, `/home`)

#### Memory
Monitors system memory usage:

| Metric                | Description       |
|-----------------------|-------------------|
| `mem_total_bytes`     | Total memory      |
| `mem_available_bytes` | Available memory  |
| `mem_used_bytes`      | Used memory       |
| `mem_free_bytes`      | Free memory       |
| `mem_used_ratio`      | Used memory ratio |

#### Network
Monitors network interface statistics:

| Metric                  | Description                         |
|-------------------------|-------------------------------------|
| `net_bytes_sent_bps`    | Bytes sent per second               |
| `net_bytes_recv_bps`    | Bytes received per second           |
| `net_packets_sent_rate` | Packets sent per second             |
| `net_packets_recv_rate` | Packets received per second         |
| `net_errin_rate`        | Incoming errors per second          |
| `net_errout_rate`       | Outgoing errors per second          |
| `net_dropin_rate`       | Incoming dropped packets per second |
| `net_dropout_rate`      | Outgoing dropped packets per second |

**Tags:**
- `interface`: The network interface name  (e.g., `eth0`, `wlan0`)

### Logs

#### Nginx
Default NGINX log files (`/var/log/nginx/*.log`),