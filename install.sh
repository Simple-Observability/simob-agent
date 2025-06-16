#!/bin/bash

# Stop the execution if one of the following commands has an error.
# The default shell behavior is to ignore errors and continue.
set -e

# Define constants
SERVICE_NAME="simob"
CUSTOM_USER="simob-agent"
CUSTOM_GROUP="simob-admins"
INSTALL_DIR="/opt/simob"
INSTALL_PATH="$INSTALL_DIR"/"$SERVICE_NAME"
SERVICE_FILE_PATH="/etc/systemd/system/${SERVICE_NAME}.service"

# Check if API key argument is provided
if [[ -z "$1" ]]; then
  echo "[x] Usage: sudo ./install.sh <API_KEY>"
  exit 1
fi
API_KEY="$1"

# Ensure this install script is run as root.
if [[ "$EUID" -ne 0 ]]; then
  echo "[x] This installer needs to be run as root. Use sudo."
  exit 1
fi

# Capture the invoking user with the SUDO_USER environment variable.
# $(whoami) is a fallback and ensures it still works if the script is somehow run directly as root
REAL_USER=${SUDO_USER:-$(whoami)}
echo "[*] Install script invocked by $REAL_USER"

if [[ -z "${BINARY_PATH}" ]]; then
  # Download the last version of the prebuilt binary if BINARY_PATH is not defined
  OS="$(uname | tr '[:upper:]' '[:lower:]')"
  ARCH="$(uname -m)"
  if [[ "$ARCH" == "x86_64" ]]; then
    ARCH="amd64"
  fi
  BINARY_URL="https://github.com/Simple-Observability/simob-agent/releases/latest/download/simob-${OS}-${ARCH}"
  DOWNLOAD_DEST="/tmp/"
  DOWNLOAD_FILE="${DOWNLOAD_DEST}/simob-${OS}-${ARCH}"
  echo "[*] Downloading binary from $BINARY_URL to $DOWNLOAD_FILE..."
  wget -q --show-progress -O "$DOWNLOAD_FILE" "$BINARY_URL"
  export BINARY_PATH="$DOWNLOAD_FILE"
  echo "Binary downloaded to: $BINARY_PATH"
else
  # If BINARY_PATH is set, run the install process with an existing binary (either downloaded
  # or built from source)
  echo "[*] Using existing binary at: $BINARY_PATH"
fi

# Create a system user without login access or home directory
# This user will be used to run the simob agent securely via systemd
echo "[+] Creating custom user and shared group..."
id $CUSTOM_USER &>/dev/null || useradd --system --no-create-home --shell /usr/sbin/nologin $CUSTOM_USER
groupadd --force "$CUSTOM_GROUP"

# Add user to the custom simob group. This allows you to run simob as non-root because it needs
# to write to the install path
echo "[+] Adding $REAL_USER and $CUSTOM_USER to $CUSTOM_GROUP group..."
usermod -aG "$CUSTOM_GROUP" "$REAL_USER"
usermod -aG "$CUSTOM_GROUP" "$CUSTOM_USER"

# Create necessary directories and assign ownership
echo "[+] Creating directories and setting custom permissions ..."
mkdir -p "$INSTALL_DIR"
chown -R $CUSTOM_USER:$CUSTOM_GROUP "$INSTALL_DIR"
chmod -R 770 "$INSTALL_DIR"
chmod g+s "$INSTALL_DIR"

# Install the binary
echo "[+] Installing binary in $INSTALL_PATH ..."
cp "$BINARY_PATH" "$INSTALL_PATH"
chown $CUSTOM_USER:$CUSTOM_GROUP "$INSTALL_PATH"
chmod +x "$INSTALL_PATH"

# Make the binary globally accessible
echo "[+] Linking binary to /usr/local/bin/${SERVICE_NAME} ..."
ln -sf "$INSTALL_PATH" /usr/local/bin/${SERVICE_NAME}

# Create and install the systemd service unit file
echo "[+] Setting up systemd service..."
cat << EOF > "${SERVICE_FILE_PATH}"
[Unit]
Description=${SERVICE_NAME} daemon
After=network.target

[Service]
Type=simple
ExecStart=${INSTALL_PATH} start
Restart=on-failure
User=${CUSTOM_USER}
Group=${CUSTOM_GROUP}

# Grant read/search access to the filesystem (bypassing some permission checks)
AmbientCapabilities=CAP_DAC_READ_SEARCH
CapabilityBoundingSet=CAP_DAC_READ_SEARCH

# Defense-in-depth hardening
NoNewPrivileges=yes           # Prevent gaining any further privileges
ProtectSystem=strict          # Mount /usr, /boot, /etc read-only
ProtectHome=yes               # Isolate /home, /root, /run/user
PrivateTmp=true               # Private /tmp and /var/tmp

[Install]
WantedBy=multi-user.target
EOF

# Apply the new systemd configuration and finish installation
# The if/else statement is meant for systems that are not running systemd
if pidof systemd > /dev/null; then
  echo "[+] Reloading systemd ..."
  systemctl daemon-reload
else
  echo "[!] Skipping systemctl setup: not running under systemd."
fi

echo "[+] Install is done."

echo "[*] Checking simob version..."
sudo -u $CUSTOM_USER "$INSTALL_PATH" version

echo "[*] Initializing simob with provided API key..."
sudo -u $CUSTOM_USER "$INSTALL_PATH" init "$API_KEY" "${@:2}"
echo "[+] Initialization complete."

echo "[*] Starting simob systemd service..."
systemctl start simob.service
echo "[+] simob service started."

echo ""
echo "[*] Simple Observability (simob) agent has been installed and initialized successfully."
echo ""
echo "[*] You can now manage the service with systemctl, for example:"
echo "    systemctl start $SERVICE_NAME"
echo "    systemctl status $SERVICE_NAME"
echo ""

echo "[*] To run the simob manually, first make sure you have updated your group membership:"
echo "    Log out and back in, or run 'newgrp $CUSTOM_GROUP' to apply group changes."
echo ""
echo "    Then you can simply run commands like:"
echo "      simob help"
echo "      simob <other-command>"
echo ""
