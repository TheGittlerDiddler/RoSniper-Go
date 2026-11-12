# <img width="32" height="32" alt="RoSniper-Go" src="https://github.com/user-attachments/assets/628445a0-5e54-41b6-ba5e-dbe9ab6864e0" /> RoSniper-Go

![GitHub Release](https://img.shields.io/github/v/release/TheGittlerDiddler/RoSniper-Go)
![GitHub License](https://img.shields.io/github/license/TheGittlerDiddler/RoSniper-Go)

A Roblox player presence tracker and automated game joiner built with a terminal user interface (TUI) in Go.

RoSniper queries Roblox's Presence API to monitor when a target user enters an experience. When a joinable server instance (Job ID) is detected, the program generates an authentication ticket and launches the Roblox desktop client directly into that server.

**Disclaimer:** You can only join users who allow you to join them (e.g., public joins are enabled, or the target is a friend/followed user). RoSniper does not bypass privacy settings, inject into game memory, or operate as an exploit.

---

## API Rate Limit Notice

Roblox's cookie-authenticated Presence API enforces a rate limit of **100 requests per minute** (~0.6 seconds minimum interval per request). 

To prevent HTTP 429 rate-limiting, RoSniper defaults to a **1.0-second** request interval. If the endpoint returns a 429 status, the client automatically backs off for the duration specified in the `Retry-After` header before resuming.

Reference: [Roblox Developer Forum: Cookie-Based API Updates](https://devforum.roblox.com/t/roblox-data-stewardship-principles-cookie-based-api-update/4579220)

---

## Features

- **Terminal Interface**: Interactive interface built with Bubble Tea and Lipgloss.
- **Direct Protocol Dispatch**: Generates single-use authentication launch tickets (`roblox-player:` protocol) with a fallback to deep linking (`roblox://`).
- **Parallel Cookie Validation**: Validates multiple `.ROBLOSECURITY` account cookies concurrently on startup using goroutines.
- **Multi-Account Switching**: Store multiple account sessions and switch active accounts directly from the interface.
- **Batch Monitoring**: Queries up to 50 users in a single HTTP request to monitor player activity without consuming extra rate-limit quota.
- **Skip Initial Server**: Optional toggle to ignore the first detected server instance to avoid joining full rooms when a streamer is already in an older server.
- **Target History**: Saves recent usernames to configuration for faster access.
- **Customizable Controls**: Modify navigation, confirm, and cancel keybinds from within the settings menu.
- **Flexible Path Resolution**: Operates portably if `config.json` is located next to the executable; otherwise, stores configuration in the user configuration directory (`%APPDATA%\rosniper\` on Windows, `~/.config/rosniper/` on macOS/Linux).

---

## Default Controls

| Action | Primary Key | Alternative Key |
| :--- | :--- | :--- |
| Move Up | `W` | `Up Arrow` |
| Move Down | `S` | `Down Arrow` |
| Confirm / Select | `V` | `Enter` |
| Deny / Back / Stop | `B` | `Esc` |
| Quit Application | `Ctrl + C` | |

*Note: Inside text input prompts (usernames, cookies), letter keys type standard text. `Enter` submits the input, and `Esc` cancels back to the menu.*

---

## Installation

### Pre-compiled Binaries (Recommended)

Pre-built standalone executables for Windows, macOS, and Linux are available under the [Releases](https://github.com/TheGittlerDiddler/RoSniper-Go/releases) section. Download the binary for your platform and run it directly. No installation or external dependencies are required.

### Building from Source

#### Prerequisites

- [Go 1.22+](https://go.dev/dl/)

#### Build Commands

Clone the repository and build the binary:

```bash
git clone https://github.com/TheGittlerDiddler/RoSniper-Go.git
cd RoSniper-Go
```

##### Windows (Standard Build)
```powershell
go build -ldflags="-s -w" -o rosniper.exe
```

##### Windows (100% Statically Linked, No C Runtime Dependencies)
```powershell
$env:CGO_ENABLED=0
go build -ldflags="-s -w -extldflags '-static'" -o rosniper.exe
```

##### macOS / Linux
```bash
CGO_ENABLED=0 go build -ldflags="-s -w" -o rosniper
```

---

## Running

```powershell
# Windows
.\rosniper.exe

# macOS / Linux
./rosniper
```

---

## Configuration

Settings and accounts are stored in `config.json`:

```json
{
  "accounts": [
    {
      "alias": "Main",
      "cookie": "_|WARNING:-DO-NOT-SHARE-THIS.--...",
      "user_id": 12345678,
      "username": "PlayerOne",
      "valid": true
    }
  ],
  "active_account": "Main",
  "request_delay_seconds": 1.0,
  "decline_first_server": false,
  "recent_targets": [
    "TargetUser"
  ],
  "keybinds": {
    "up": "w",
    "down": "s",
    "confirm": "v",
    "back": "b"
  }
}
```

---

## License

This project is licensed under the [GNU General Public License v3.0](LICENSE).
