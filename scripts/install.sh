#!/bin/bash
set -e

INSTALL_DIR="$HOME/.local/share/voxi"
BIN_DIR="$HOME/.local/bin"
SERVICE_FILE="$HOME/.config/systemd/user/voxi.service"

echo "=============================================="
echo "Voxi Installation"
echo "=============================================="

# ---------------------------------------------------------------------------
# 1. System dependencies (requires sudo)
# ---------------------------------------------------------------------------
echo ""
echo "[1/8] Installing system dependencies..."
echo "      (You may be prompted for your sudo password)"
sudo apt update
sudo apt install -y libportaudio2 wl-clipboard xclip libnotify-bin
echo "      Done."

# ---------------------------------------------------------------------------
# 2. Install uv if not present
# ---------------------------------------------------------------------------
echo ""
echo "[2/8] Checking for uv..."
if command -v uv &>/dev/null; then
    echo "      uv is already installed."
else
    echo "      Installing uv..."
    curl -LsSf https://astral.sh/uv/install.sh | sh
    # Add to PATH for this session
    export PATH="$HOME/.local/bin:$PATH"
    echo "      Done."
fi

# ---------------------------------------------------------------------------
# 3. Install Python 3.11
# ---------------------------------------------------------------------------
echo ""
echo "[3/8] Installing Python 3.11..."
uv python install 3.11
echo "      Done."

# ---------------------------------------------------------------------------
# 4. Copy project to permanent location
# ---------------------------------------------------------------------------
echo ""
echo "[4/8] Syncing project to $INSTALL_DIR..."
mkdir -p "$INSTALL_DIR"
# Use rsync to mirror code, ignoring local .venv and .git to prevent copying large/unnecessary files
rsync -a --delete --exclude='.git' --exclude='.venv' "$(pwd)/" "$INSTALL_DIR/"
echo "      Done."

# ---------------------------------------------------------------------------
# 5. Install Python dependencies
# ---------------------------------------------------------------------------
echo ""
echo "[5/8] Installing Python dependencies in $INSTALL_DIR..."
cd "$INSTALL_DIR"
uv sync
cd - > /dev/null
echo "      Done."

# ---------------------------------------------------------------------------
# 6. Create ~/.local/bin and symlink client
# ---------------------------------------------------------------------------
echo ""
echo "[6/8] Setting up client symlink..."
# Create ~/.local/bin if it doesn't exist (requires sudo for parent dir)
if [ ! -d "$BIN_DIR" ]; then
    sudo mkdir -p "$BIN_DIR"
    sudo chown "$USER:$USER" "$BIN_DIR"
    echo "      Created $BIN_DIR"
fi
ln -sf "$INSTALL_DIR/client/voxi-toggle.py" "$BIN_DIR/voxi-toggle"
echo "      Symlinked voxi-toggle to $BIN_DIR"
echo "      Done."

# ---------------------------------------------------------------------------
# 7. Symlink systemd service
# ---------------------------------------------------------------------------
echo ""
echo "[7/8] Setting up systemd service..."
mkdir -p "$(dirname "$SERVICE_FILE")"
ln -sf "$INSTALL_DIR/systemd/voxi.service" "$SERVICE_FILE"
echo "      Symlinked voxi.service to $SERVICE_FILE"
echo "      Done."

# ---------------------------------------------------------------------------
# 8. Enable and restart service
# ---------------------------------------------------------------------------
echo ""
echo "[8/8] Enabling and restarting Voxi service..."
systemctl --user daemon-reload
systemctl --user enable voxi.service
systemctl --user restart voxi.service
echo "      Done."

echo ""
echo "=============================================="
echo "Voxi is installed and running!"
echo ""
echo "To check status:"
echo "  systemctl --user status voxi.service"
echo ""
echo "To view logs:"
echo "  journalctl --user -u voxi.service -f"
echo ""
echo "Bind a GNOME keyboard shortcut to:"
echo "  voxi-toggle"
echo "=============================================="
