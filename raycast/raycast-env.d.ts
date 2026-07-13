/// <reference types="@raycast/api">

/* 🚧 🚧 🚧
 * This file is auto-generated from the extension's manifest.
 * Do not modify manually. Instead, update the `package.json` file.
 * 🚧 🚧 🚧 */

/* eslint-disable @typescript-eslint/ban-types */

type ExtensionPreferences = {
  /** ssht Path - Path to the ssht executable. Use an absolute path if Raycast cannot find ssht on PATH. */
  "sshtPath": string,
  /** SSH Config Path - Optional path to an OpenSSH config file. Leave empty to use ~/.ssh/config. */
  "configPath"?: string,
  /** Disable Include Parsing - Ignore Include directives while listing hosts. */
  "noInclude": boolean,
  /** Terminal - Terminal backend used by ssht when opening a connection. */
  "terminal": "auto" | "iterm" | "terminal" | "wezterm" | "kitty" | "alacritty" | "ghostty" | "warp",
  /** Open Mode - Whether ssht should use automatic iTerm split detection, a new tab, or a new window. */
  "openMode": "auto" | "tab" | "window"
}

/** Preferences accessible in all the extension's commands */
declare type Preferences = ExtensionPreferences

declare namespace Preferences {
  /** Preferences accessible in the `search-hosts` command */
  export type SearchHosts = ExtensionPreferences & {}
}

declare namespace Arguments {
  /** Arguments passed to the `search-hosts` command */
  export type SearchHosts = {}
}
