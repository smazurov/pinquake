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

### Picking a version

Pass a version to pin the install, or `dev` for the rolling build from main. Downgrades work the same way.

```sh
curl -fsSL https://raw.githubusercontent.com/smazurov/pinquake/main/install.sh | bash -s -- 1.4.0
curl -fsSL https://raw.githubusercontent.com/smazurov/pinquake/main/install.sh | bash -s -- dev
```

## Let OBS show and hide the overlays (optional)

By default the overlays hide themselves when the trigger fades. PinQuake can instead toggle a source or group in OBS, so you can use OBS show/hide transitions or hide several sources together:

1. In OBS, enable **Tools → WebSocket Server Settings** and note the port and password.
2. In the **OBS** panel on the config page, enter the server (`localhost:4455`) and the password if authentication is on, then **Connect**.
3. Pick the source or group to show and hide.

While connected, the overlays stay drawn and OBS hides the target. If OBS goes away, they hide themselves again until it's back. The password is stored in plaintext in `config.toml`. Keep **Shutdown source when not visible** off on PinQuake browser sources so they don't reload on every trigger.

## Local development

Prerequisites: Go, [pnpm](https://pnpm.io), [air](https://github.com/air-verse/air), [process-compose](https://github.com/F1bonacc1/process-compose)

```sh
process-compose up
```

Runs the Go backend with air (live reload) and the Vite dev server for the UI.
