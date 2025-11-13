#!/bin/bash

# Stop the execution if one of the following commands has an error.
# The default shell behavior is to ignore errors and continue.
set -e

# Define constants
SERVICE_NAME="simob"
CUSTOM_USER="simob-agent"
CUSTOM_GROUP="simob-admins"
SERVICE_FILE_PATH="/etc/systemd/system/${SERVICE_NAME}.service"

# Sends a minimal exit reason payload to a telemetry endpoint before exiting the current script
exit_with_telemetry() {
  if [[ "$SKIP_TELEMETRY" == "1" ]]; then
    exit 1
  fi

  local exit_reason="$1"
  local TELEMETRY_ENDPOINT="https://api.simpleobservability.com/telemetry/install"
  if [[ -z "$exit_reason" ]]; then
    exit_reason="Unspecified reason"
  fi
  local PAYLOAD
  PAYLOAD=$(printf '{"reason": "%s"}' "${exit_reason}")
  curl -s -m 5 -X POST -H "Content-Type: application/json" --data "$PAYLOAD" "$TELEMETRY_ENDPOINT"
  exit 1
}

# Checks if the 'curl' command-line tool is installed and available in the PATH.
check_dependencies() {
  if ! command -v curl &> /dev/null; then
    echo "[x] Error: The 'curl' command is not installed."
    echo "    Please install curl (e.g., using 'apt', 'yum', or 'brew') and try again."
    exit_with_telemetry "'curl' command not installed"
  fi
}

# Validates the provided API key against the remote backend via cURL.
check_key_validity() {
  local key="$1"
  echo "[*] Validating API key on remote server..."
  HTTP_CODE=$(
    curl -s \
    -o /dev/null \
    -w "%{http_code}" \
    -H "Content-Type: application/json" \
    -d "{\"api_key\": \"$key\"}" \
    "https://api.simpleobservability.com/check-key/"
  )
  if [ "$HTTP_CODE" -eq 200 ]; then
    echo "[+] API key is valid."
  else
    echo "[x] Error: API key check failed. Received HTTP $HTTP_CODE."
    exit_with_telemetry "API key is not valid"
  fi
}

# Fetch the agent binary
fetch_binary() {
  if [[ -z "${BINARY_PATH}" ]]; then
    # Download the last version of the prebuilt binary if BINARY_PATH is not defined
    OS="$(uname | tr '[:upper:]' '[:lower:]')"
    ARCH="$(uname -m)"
    case "$ARCH" in
      x86_64)
        ARCH="amd64"
        ;;
      aarch64)
        ARCH="arm64"
        ;;
      *)
        echo "[x] Unsupported architecture: $ARCH" >&2
        exit_with_telemetry "'$ARCH' is not a supported architecture"
        ;;
    esac
    BINARY_URL="https://github.com/Simple-Observability/simob-agent/releases/latest/download/simob-${OS}-${ARCH}"
    DOWNLOAD_DEST="/tmp"
    DOWNLOAD_FILE="${DOWNLOAD_DEST}/simob-${OS}-${ARCH}"
    echo "[*] Downloading binary from $BINARY_URL to $DOWNLOAD_FILE..."
    curl -# -L -o "$DOWNLOAD_FILE" "$BINARY_URL"
    export BINARY_PATH="$DOWNLOAD_FILE"
    echo "[+] Binary downloaded to: $BINARY_PATH"
  else
    # If BINARY_PATH is set, run the install process with an existing binary (either downloaded
    # or built from source)
    echo "[*] Using existing binary at: $BINARY_PATH"
  fi
}

# Set up a systemd service
setup_systemd_service() {
  # Create and install the systemd service unit file
  echo "[+] Setting up systemd service..."

  # Optional system-wide read capability (default: enabled)
  local SYSTEM_CAPABILITIES=""
  if [ "${NO_SYSTEM_READ}" = false ]; then
    SYSTEM_CAPABILITIES="
# Grant read/search access to the filesystem (bypassing some permission checks)
AmbientCapabilities=CAP_DAC_READ_SEARCH
CapabilityBoundingSet=CAP_DAC_READ_SEARCH
    "
  fi

  # Create service file
  cat <<EOF > "${SERVICE_FILE_PATH}"
[Unit]
Description=${SERVICE_NAME} daemon
After=network.target

[Service]
Type=simple
ExecStart=${INSTALL_PATH} start
Restart=always
User=${CUSTOM_USER}
Group=${CUSTOM_GROUP}
${SYSTEM_CAPABILITIES}
# Prevent gaining any further privileges
NoNewPrivileges=yes
# Mount /usr, /boot, /etc read-only
ProtectSystem=full
# Isolate /home, /root, /run/user
ProtectHome=yes
# Private /tmp and /var/tmp
PrivateTmp=true

[Install]
WantedBy=multi-user.target
EOF

  echo "[+] Reloading systemd daemon ..."
  systemctl daemon-reload

  echo "[*] Starting systemd service..."
  systemctl enable "${SERVICE_NAME}.service"
  systemctl start "${SERVICE_NAME}.service"
  echo "[+] Service started and enabled."
}

# Create a dedicated user to run simob as a systemd service.
create_custom_user() {
  # Create a system user without login access or home directory
  # This user will be used to run the simob agent securely via systemd
  echo "[+] Creating custom user and shared group..."
  id $CUSTOM_USER &>/dev/null || useradd --system --no-create-home --shell /usr/sbin/nologin $CUSTOM_USER
  groupadd --force "$CUSTOM_GROUP"

  # Capture the invoking user with the SUDO_USER environment variable.
  # $(whoami) is a fallback and ensures it still works if the script is somehow run directly as root
  REAL_USER=${SUDO_USER:-$(whoami)}

  # Add user to the custom simob group. This allows you to run simob as non-root because it needs
  # to write to the install path
  echo "[+] Adding $REAL_USER and $CUSTOM_USER to $CUSTOM_GROUP group..."
  usermod -aG "$CUSTOM_GROUP" "$REAL_USER"
  usermod -aG "$CUSTOM_GROUP" "$CUSTOM_USER"

  # Set appropriate permissions
  chown -R "$CUSTOM_USER":"$CUSTOM_GROUP" "$INSTALL_DIR"
  chmod -R 770 "$INSTALL_DIR"
  chmod g+s "$INSTALL_DIR"
  chown "$CUSTOM_USER":"$CUSTOM_GROUP" "$INSTALL_PATH"
}

# -------------------- Parse options --------------------
# Default values
NO_SYSTEM_READ=false
NO_JOURNAL_ACCESS=false
SKIP_KEY_CHECK=false
API_KEY=""
EXTRA_ARGS=()

for arg in "$@"; do
  case "$arg" in
    --no-system-read)
      NO_SYSTEM_READ=true
      shift
      ;;
    --no-journal-access)
      NO_JOURNAL_ACCESS=true
      shift
      ;;
    --skip-key-check)
      SKIP_KEY_CHECK=true
      shift
      ;;
    --)
      shift
      EXTRA_ARGS=("$@")  # everything after -- goes here
      break
      ;;
    *)
      if [[ -z "$API_KEY" ]]; then
        API_KEY="$arg"
        shift
      else
        echo "[x] Unexpected extra argument: $arg"
        echo "Usage: sudo install.sh <API_KEY> [--no-system-read] [--no-journal-access]"
        exit_with_telemetry "Unexpected extra arguments"
      fi
      ;;
  esac
done

# Check if API key argument was provided
if [[ -z "$API_KEY" ]]; then
  echo "[x] Missing API key"
  echo "Usage: sudo install.sh <API_KEY> [--no-system-read] [--no-journal-access]"
  exit_with_telemetry "API key is missing"
fi
# -------------------------------------------------------

# Dependency check
check_dependencies

# Check API key, unless skip flag is set
if [[ "$SKIP_KEY_CHECK" == "false" ]]; then
  check_key_validity "$API_KEY"
fi

# Check if system is running under systemd
if pidof systemd > /dev/null; then
  SKIP_SYSTEMD=false
else
  echo "[!] Skipping systemctl setup: not running under systemd."
  SKIP_SYSTEMD=true
fi

# Check if install is running as sudo
if [[ "$EUID" -eq 0 ]]; then
  echo "[*] Running install in sudo mode."
  IS_SUDO_MODE="true"
  INSTALL_DIR="/opt/simob"
  LINK_DIR="/usr/local/bin"
else
  echo "[*] Running install in non-sudo mode."
  IS_SUDO_MODE="false"
  INSTALL_DIR="$HOME/.local/simob"
  LINK_DIR="$HOME/.local/bin"
fi

# Fetch binary
fetch_binary

# Install the binary
echo "[+] Installing binary in $INSTALL_DIR ..."
mkdir -p "$INSTALL_DIR" "$LINK_DIR"
INSTALL_PATH="$INSTALL_DIR"/"$SERVICE_NAME"
cp "$BINARY_PATH" "$INSTALL_PATH"
ln -sf "$INSTALL_DIR/$SERVICE_NAME" "$LINK_DIR/$SERVICE_NAME"
chmod +x "$INSTALL_PATH"

if [[ "$IS_SUDO_MODE" == "true" ]]; then
  create_custom_user

  # Conditionally add the simob user to the "systemd-journal" group
  if [[ "$NO_JOURNAL_ACCESS" == false ]]; then
    echo "[+] Granting journal access to $CUSTOM_USER..."
    usermod -aG systemd-journal "$CUSTOM_USER"
  else
    echo "[*] Skipping journal access for $CUSTOM_USER (--no-journal-access flag set)"
  fi

fi

echo "[*] Initializing simob with provided API key..."
if [[ "$IS_SUDO_MODE" == "true" ]]; then
  sudo -u "$CUSTOM_USER" "$INSTALL_PATH" init "$API_KEY" "${EXTRA_ARGS[@]}"
else
  $INSTALL_PATH init "$API_KEY" "${EXTRA_ARGS[@]}"
fi
echo "[+] Initialization complete."

# Apply the new systemd configuration and finish installation
if [ "${SKIP_SYSTEMD}" = false ]; then
  if [[ "$IS_SUDO_MODE" == "true" ]]; then
    setup_systemd_service
  fi
else
  echo "[!] Skipping service setup as systemd is not running."
fi

echo ""
echo "[*] Simple Observability (simob) agent has been installed and initialized successfully."
echo ""

if [ "${SKIP_SYSTEMD}" = false ]; then
  if [[ "$IS_SUDO_MODE" == "true" ]]; then
  echo "[*] The **system-wide service** is running as user '$CUSTOM_USER' and is enabled for startup."
  echo "[*] You can manage the service using global 'systemctl', for example:"
  echo "    sudo systemctl status $SERVICE_NAME"
  echo "    sudo systemctl restart $SERVICE_NAME"
  echo ""
  echo "[*] To run 'simob' commands manually, you need to update your group membership:"
  echo "    Log out and back in, or run 'newgrp $CUSTOM_GROUP' to refresh permissions."

  else
    echo "[*] You chose to install without 'sudo'."
    echo "    The agent has been initialized successfully, but no system service was created automatically."
    echo ""
    echo "[*] To run the agent manually, use:"
    echo "    simob start"
    echo ""
  fi
else
  echo "[!] Service management (systemd) was skipped as it was not detected on this system."
  echo ""
  if [[ "$IS_SUDO_MODE" == "true" ]]; then
    echo "[*] The agent was installed to '$INSTALL_PATH' and requires a group refresh for manual use."
    echo "    Log out and back in, or run 'newgrp $CUSTOM_GROUP' to apply group changes."
    echo "    Then you can run the agent manually: 'simob start'"
  else
    echo "[*] The agent was installed to '$INSTALL_PATH' and symlinked to '$LINK_DIR'."
    echo "    Ensure '$LINK_DIR' is in your \$PATH."
  fi
  echo ""
fi
