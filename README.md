A retro terminal music player inspired by Winamp. Play local files, streams, podcasts, YouTube, YouTube Music, SoundCloud, Bilibili, Spotify, NetEase Cloud Music, Xiaoyuzhou (小宇宙), Navidrome, Plex, and Jellyfin with a spectrum visualizer, parametric EQ, and playlist management.

**[clawdamp.stream](https://clawdamp.stream)**

Built with [Bubbletea](https://github.com/charmbracelet/bubbletea), [Lip Gloss](https://github.com/charmbracelet/lipgloss), [Beep](https://github.com/gopxl/beep), and [go-librespot](https://github.com/devgianlu/go-librespot).


https://github.com/user-attachments/assets/fbc33d20-e3ac-4a62-a991-8a2f0243c8ea


## Install

```sh
curl -fsSL https://raw.githubusercontent.com/clawd-fi/clawdamp/HEAD/install.sh | sh
```

**Homebrew**

```sh
brew install clawd-fi/clawdamp/clawdamp
```

The formula pulls in all required runtime libraries automatically.

**Arch Linux (AUR)**

```sh
yay -S clawdamp
```

**Pre-built binaries**

Download from [GitHub Releases](https://github.com/clawd-fi/clawdamp/releases/latest).

> **macOS:** the pre-built binaries dynamically link against FLAC, Vorbis, and Ogg
> from Homebrew. If you download directly from Releases (or use the `install.sh`
> script) you must install them first, otherwise you will see errors like
> `Library not loaded: /opt/homebrew/opt/libvorbis/lib/libvorbisenc.2.dylib`:
>
> ```sh
> brew install flac libvorbis libogg
> ```
>
> Installing via `brew install clawd-fi/clawdamp/clawdamp` does this for you.
>
> **Linux:** the pre-built binaries statically link FLAC, Vorbis, and Ogg, so no
> extra codec packages are required. You may still need an ALSA bridge for your
> sound server — see [Troubleshooting](#troubleshooting).
>
> **Windows:** download `clawdamp-windows-amd64.exe` from Releases. If `HOME` is not
> set, clawdamp stores its config under `%APPDATA%\clawdamp`. The Spotify provider is
> currently unavailable on Windows builds.

**Optional runtime dependencies** (all platforms, all install methods):

- [ffmpeg](https://ffmpeg.org/) — for AAC, ALAC, Opus, and WMA playback
- [yt-dlp](https://github.com/yt-dlp/yt-dlp) — for YouTube, YouTube Music, SoundCloud, Bandcamp, Bilibili, and NetEase Cloud Music

On macOS: `brew install ffmpeg yt-dlp`. On Linux, use your distribution's package manager.

On Windows, install `ffmpeg` and `yt-dlp` with your preferred package manager and keep both on `PATH`.

**Build from source**

```sh
git clone https://github.com/clawd-fi/clawdamp.git && cd clawdamp && go build -o clawdamp .
```

## Quick Start

```sh
clawdamp ~/Music                     # play a directory
clawdamp *.mp3 *.flac               # play files
clawdamp https://example.com/stream  # play a URL
```

Press `Ctrl+K` to see all keybindings.

**Configure remote providers** (Navidrome, Plex, Jellyfin, Spotify, YouTube Music, NetEase Cloud Music) with the interactive wizard:

```sh
clawdamp setup
```

It walks you through each provider, validates the connection, and writes the right block to your config file (`~/.config/clawdamp/config.toml`, or `%APPDATA%\clawdamp\config.toml` on Windows when `HOME` is unset). See [docs/cli.md](docs/cli.md#setup-wizard) for details.

## Radio

Press `R` in the player to browse and search 30,000+ online radio stations from the [Radio Browser](https://www.radio-browser.info/) directory.

Add your own stations to `~/.config/clawdamp/radios.toml` (or `%APPDATA%\clawdamp\radios.toml` on Windows when `HOME` is unset). See [docs/configuration.md](docs/configuration.md#custom-radio-stations).

Want to host your own radio? Check out [clawdamp-server](https://github.com/clawd-fi/clawdamp-server).

## Building from source

**Prerequisites:**

- [Go](https://go.dev/dl/) 1.25.5 or later
- ALSA development headers (Linux only — required by the audio backend)

**Linux (Debian/Ubuntu):**

```sh
sudo apt install libasound2-dev
```

**Linux (Fedora):**

```sh
sudo dnf install alsa-lib-devel libvorbis-devel flac-devel
```

**Linux (Arch):**

```sh
sudo pacman -S alsa-lib
```

**macOS:** No extra dependencies — CoreAudio is used.

**Windows:** No extra SDKs required for the core player. `ffmpeg.exe` and `yt-dlp.exe` remain optional runtime dependencies for the same formats/providers as on other platforms. Spotify is not available on Windows builds.

**Clone and build:**

```sh
git clone https://github.com/clawd-fi/clawdamp.git
cd clawdamp
make && make install
```

Or without Make: `go build -o clawdamp .`

`make install` places the binary in `~/.local/bin/`.

**Optional runtime dependencies:**

- [ffmpeg](https://ffmpeg.org/) — for AAC, ALAC, Opus, and WMA playback
- [yt-dlp](https://github.com/yt-dlp/yt-dlp) — for YouTube, SoundCloud, Bandcamp, Bilibili, and NetEase Cloud Music

## Docs

- [Configuration](docs/configuration.md)
- [Keybindings](docs/keybindings.md)
- [CLI Flags](docs/cli.md)
- [Streaming](docs/streaming.md)
- [Playlists](docs/playlists.md)
- [YouTube, SoundCloud, Bandcamp and Bilibili](docs/yt-dlp.md)
- [YouTube Music](docs/youtube-music.md)
- [NetEase Cloud Music](docs/netease.md)
- [SoundCloud](docs/soundcloud.md)
- [Lyrics](docs/lyrics.md)
- [Spotify](docs/spotify.md)
- [Navidrome](docs/navidrome.md)
- [Plex](docs/plex.md)
- [Jellyfin](docs/jellyfin.md)
- [Themes](docs/themes.md)
- [SSH Streaming](docs/ssh-streaming.md)
- [Remote Control (IPC)](docs/remote-control.md)
- [Headless Daemon Mode](docs/headless.md)
- [Audio Quality](docs/audio-quality.md)
- [Media Controls](docs/mediactl.md)
- [Quickshell Now-Playing Widget (Omarchy)](docs/quickshell.md)
- [Lua Plugins](docs/plugins.md)
  - [Community Plugins](docs/community-plugins.md)
  - [Soap Bubbles Visualizer](https://github.com/clawd-fi/clawdamp-plugin-soap-bubbles)

## Troubleshooting

**No audio output (silence with no errors)**

On Linux systems using PipeWire or PulseAudio, clawdamp's ALSA backend needs a bridge package to route audio through your sound server:

- **PipeWire:** `pipewire-alsa`
- **PulseAudio:** `pulseaudio-alsa`

Install the appropriate package for your system:

```sh
# PipeWire (Arch)
sudo pacman -S pipewire-alsa

# PulseAudio (Arch)
sudo pacman -S pulseaudio-alsa

# Debian/Ubuntu (PipeWire)
sudo apt install pipewire-alsa
```

## Author

[x.com/iamdothash](https://x.com/iamdothash)

## Disclaimer

Use this software at your own risk. We are not responsible for any damages or issues that may arise from using this software.
