# PinQuake

Real-time pinball machine vibration visualizer for live streams. Connects to a BLE accelerometer sensor mounted on the machine and renders force data as transparent browser-source overlays (waveform and crosshair) that you can add to OBS.

Supports WT901 and PinLevel BLE sensors.

## Install

Linux (amd64/arm64):

```sh
curl -fsSL https://raw.githubusercontent.com/smazurov/pinquake/main/install.sh | bash
```

Installs the binary to `~/.local/bin`, sets up config at `~/.config/pinquake`, and creates a systemd user service.

```sh
systemctl --user start pinquake    # start
journalctl --user -u pinquake -f   # logs
```

Open `http://localhost:8091` to configure sensor connection, visualization settings, and grab the overlay URLs for OBS.

## Local development

Prerequisites: Go, [pnpm](https://pnpm.io), [air](https://github.com/air-verse/air), [process-compose](https://github.com/F1bonacc1/process-compose)

```sh
process-compose up
```

Runs the Go backend with air (live reload) and the Vite dev server for the UI.
