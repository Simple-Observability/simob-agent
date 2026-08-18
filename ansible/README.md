# simpleobservability.agent

Ansible collection for installing and managing the [Simple Observability](https://simpleobservability.com) agent (`simob`) across a fleet of servers.

## Installation

```bash
ansible-galaxy collection install simpleobservability.agent
```

## Quick start

```yaml
- hosts: all
  vars:
    simob_deploy_key: "{{ vault_deploy_key }}"
  roles:
    - simpleobservability.agent.simob
```

That's it. The role auto-provisions each server on the backend using the deploy key, installs the agent, and starts the service. Server names default to the system hostname.

## Variables

| Variable | Default | Description |
| :--- | :--- | :--- |
| `simob_deploy_key` | `""` | Deploy key for auto-provisioning servers. **Required.** |
| `simob_state` | `present` | `present` to install, `absent` to uninstall. |
| `simob_install_flags` | `""` | Extra flags for the install script (Linux only). Example: `--no-journal-access`. |
| `simob_skip_telemetry` | `false` | Skip anonymized install telemetry. |

## Getting a deploy key

Generate a deploy key from your Simple Observability workspace settings. A single deploy key works for all servers in your workspace. The agent auto-provisions a new server on first contact, so you don't need to pre-create servers in the UI.

## Requirements

- Ansible 2.14+
- `curl` and `sudo` on Linux targets
- Administrator access on Windows targets
- `ansible.windows` collection for Windows support

## License

MIT
