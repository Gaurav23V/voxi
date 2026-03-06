# Voxi

Voxi is a local Linux dictation tool for GNOME on Fedora. Press a GNOME keyboard shortcut that runs `voxi toggle`, speak, press it again, and Voxi inserts cleaned text into the focused application or falls back to the clipboard.

This repository contains:

- a Go CLI and daemon
- a Python ML worker
- local-only ASR and cleanup integrations
- setup automation, tests, and systemd user service assets

## Status

This MVP is being implemented from the product and technical requirements in `prd.md` and `trd.md`.

Full setup, testing, architecture, and troubleshooting documentation are added as part of the implementation milestones.
