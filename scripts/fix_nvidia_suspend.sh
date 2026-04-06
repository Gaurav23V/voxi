#!/bin/bash
set -e

# Ensure the script is run with sudo
if [ "$EUID" -ne 0 ]; then
  echo "Please run this script with sudo."
  exit 1
fi

echo "=============================================="
echo "Nvidia Suspend Fix for Linux"
echo "=============================================="

# 1. Update GRUB configuration
echo "Configuring GRUB..."
if grep -q "nvidia.NVreg_PreserveVideoMemoryAllocations=1" /etc/default/grub; then
    echo "  -> GRUB already configured, skipping."
else
    # Safely append the parameter inside the quotes
    sed -i 's/GRUB_CMDLINE_LINUX_DEFAULT="\(.*\)"/GRUB_CMDLINE_LINUX_DEFAULT="\1 nvidia.NVreg_PreserveVideoMemoryAllocations=1"/' /etc/default/grub
    echo "  -> Appended parameter to /etc/default/grub"
fi

# 2. Rebuild GRUB
echo "Updating GRUB bootloader..."
update-grub

# 3. Enable Systemd Services
echo "Enabling Nvidia Suspend Services..."
systemctl enable nvidia-suspend.service nvidia-hibernate.service nvidia-resume.service

echo "=============================================="
echo "Done! Please REBOOT your computer for the changes to take effect."
echo "=============================================="