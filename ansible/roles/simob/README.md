# simob

Installs and manages the [Simple Observability](https://simpleobservability.com) agent (`simob`) on Linux and Windows hosts.

## Requirements

- Ansible 2.14+
- `curl` and `sudo` on Linux targets
- Administrator access on Windows targets
- `ansible.windows` collection for Windows support

## Variables

| Variable | Default | Description |
| :--- | :--- | :--- |
| `simob_deploy_key` | `""` | Deploy key for auto-provisioning servers. **Required.** |
| `simob_state` | `present` | `present` to install, `absent` to uninstall. |
| `simob_install_flags` | `""` | Extra flags for the install script (Linux only). Example: `--no-journal-access`. |
| `simob_skip_telemetry` | `false` | Skip anonymized install telemetry. |

## Example playbook

```yaml
- hosts: all
  vars:
    simob_deploy_key: "{{ vault_deploy_key }}"
  roles:
    - simpleobservability.agent.simob
```

## License

MIT
