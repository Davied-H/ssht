import {
  Action,
  ActionPanel,
  Detail,
  Icon,
  List,
  Toast,
  closeMainWindow,
  getPreferenceValues,
  openExtensionPreferences,
  showToast,
} from "@raycast/api";
import { execFile } from "node:child_process";
import os from "node:os";
import path from "node:path";
import { useCallback, useEffect, useMemo, useState } from "react";

type Preferences = {
  sshtPath?: string;
  configPath?: string;
  noInclude?: boolean;
  terminal?: string;
  openMode?: string;
};

type HostEntry = {
  alias: string;
  group?: string;
  hostName?: string;
  user?: string;
  port?: string;
  identityFile?: string;
  proxyJump?: string;
  proxyCommand?: string;
  tags?: string[];
  sourceFile: string;
  sourceLine: number;
  rawBlock?: string;
};

type RunResult = {
  stdout: string;
  stderr: string;
};

class SshtError extends Error {
  stderr: string;
  stdout: string;
  code?: string | number;

  constructor(
    message: string,
    result: Partial<RunResult> & { code?: string | number },
  ) {
    super(message);
    this.name = "SshtError";
    this.stderr = result.stderr ?? "";
    this.stdout = result.stdout ?? "";
    this.code = result.code;
  }
}

export default function Command() {
  const preferences = useMemo(() => getPreferenceValues<Preferences>(), []);
  const [hosts, setHosts] = useState<HostEntry[]>([]);
  const [error, setError] = useState<string>();
  const [isLoading, setIsLoading] = useState(true);

  const loadHosts = useCallback(async () => {
    setIsLoading(true);
    setError(undefined);
    try {
      const loadedHosts = await readHosts(preferences);
      setHosts(loadedHosts);
    } catch (loadError) {
      setHosts([]);
      setError(formatError(loadError));
    } finally {
      setIsLoading(false);
    }
  }, [preferences]);

  useEffect(() => {
    void loadHosts();
  }, [loadHosts]);

  const groupedHosts = useMemo(() => groupHosts(hosts), [hosts]);

  if (error && !isLoading) {
    return (
      <Detail
        markdown={`# Could not load SSH hosts\n\n${escapeMarkdown(error)}\n\nSet **ssht Path** to the absolute path from \`./install.sh\` if Raycast cannot find \`ssht\`.`}
        actions={<RootActions onRefresh={loadHosts} />}
      />
    );
  }

  return (
    <List
      isLoading={isLoading}
      searchBarPlaceholder="Search SSH hosts by alias, hostname, user, group, or tag"
    >
      <List.EmptyView
        icon={Icon.Terminal}
        title={isLoading ? "Loading SSH Hosts" : "No SSH Hosts"}
        description={
          isLoading
            ? undefined
            : "No concrete Host entries were found in the configured SSH config."
        }
        actions={<RootActions onRefresh={loadHosts} />}
      />
      {groupedHosts.map(([group, groupHosts]) => (
        <List.Section key={group} title={group}>
          {groupHosts.map((host) => (
            <HostListItem
              key={`${host.sourceFile}:${host.sourceLine}:${host.alias}`}
              host={host}
              preferences={preferences}
              onRefresh={loadHosts}
            />
          ))}
        </List.Section>
      ))}
    </List>
  );
}

function HostListItem(props: {
  host: HostEntry;
  preferences: Preferences;
  onRefresh: () => Promise<void>;
}) {
  const { host, preferences, onRefresh } = props;
  const subtitle = formatSubtitle(host);
  const accessories: Array<{ tag: string } | { text: string }> = (
    host.tags ?? []
  )
    .slice(0, 3)
    .map((tag) => ({ tag }));
  if (host.port) {
    accessories.push({ text: `:${host.port}` });
  }

  return (
    <List.Item
      icon={Icon.Terminal}
      title={host.alias}
      subtitle={subtitle}
      keywords={hostKeywords(host)}
      accessories={accessories}
      detail={<List.Item.Detail markdown={hostMarkdown(host)} />}
      actions={
        <HostActions
          host={host}
          preferences={preferences}
          onRefresh={onRefresh}
        />
      }
    />
  );
}

function HostActions(props: {
  host: HostEntry;
  preferences: Preferences;
  onRefresh: () => Promise<void>;
}) {
  const { host, preferences, onRefresh } = props;
  return (
    <ActionPanel>
      <Action
        title="Connect"
        icon={Icon.Terminal}
        onAction={() => void connectHost(host, preferences)}
      />
      <ActionPanel.Section title="Copy">
        <Action.CopyToClipboard title="Copy Alias" content={host.alias} />
        <Action.CopyToClipboard
          title="Copy SSH Command"
          content={`ssh ${shellQuote(host.alias)}`}
        />
        {host.hostName ? (
          <Action.CopyToClipboard
            title="Copy Hostname"
            content={host.hostName}
          />
        ) : null}
        {host.user ? (
          <Action.CopyToClipboard title="Copy User" content={host.user} />
        ) : null}
        {host.sourceFile ? (
          <Action.CopyToClipboard
            title="Copy Source Path"
            content={host.sourceFile}
          />
        ) : null}
      </ActionPanel.Section>
      <ActionPanel.Section>
        <Action
          title="Refresh Hosts"
          icon={Icon.ArrowClockwise}
          shortcut={{ modifiers: ["cmd"], key: "r" }}
          onAction={() => void onRefresh()}
        />
        <Action
          title="Open Extension Preferences"
          icon={Icon.Gear}
          onAction={openExtensionPreferences}
        />
      </ActionPanel.Section>
    </ActionPanel>
  );
}

function RootActions(props: { onRefresh: () => Promise<void> }) {
  return (
    <ActionPanel>
      <Action
        title="Refresh Hosts"
        icon={Icon.ArrowClockwise}
        shortcut={{ modifiers: ["cmd"], key: "r" }}
        onAction={() => void props.onRefresh()}
      />
      <Action
        title="Open Extension Preferences"
        icon={Icon.Gear}
        onAction={openExtensionPreferences}
      />
    </ActionPanel>
  );
}

async function readHosts(preferences: Preferences): Promise<HostEntry[]> {
  const result = await runSsht(preferences, [
    ...configArgs(preferences),
    "--print-hosts",
  ]);
  try {
    const parsed = JSON.parse(result.stdout) as HostEntry[];
    if (!Array.isArray(parsed)) {
      throw new Error("expected a JSON array");
    }
    return parsed;
  } catch (error) {
    throw new SshtError(
      `ssht returned invalid JSON: ${formatError(error)}`,
      result,
    );
  }
}

async function connectHost(host: HostEntry, preferences: Preferences) {
  const toast = await showToast({
    style: Toast.Style.Animated,
    title: `Connecting ${host.alias}`,
  });
  try {
    await runSsht(preferences, [
      ...configArgs(preferences),
      "--terminal",
      normalizeText(preferences.terminal) || "auto",
      "--open-mode",
      normalizeText(preferences.openMode) || "auto",
      "--connect",
      host.alias,
    ]);
    toast.style = Toast.Style.Success;
    toast.title = `Opened ${host.alias}`;
    await closeMainWindow({ clearRootSearch: true });
  } catch (error) {
    toast.style = Toast.Style.Failure;
    toast.title = `Could not connect ${host.alias}`;
    toast.message = formatError(error);
  }
}

function runSsht(preferences: Preferences, args: string[]): Promise<RunResult> {
  const command = expandHome(normalizeText(preferences.sshtPath) || "ssht");
  const env = {
    ...process.env,
    PATH: [
      process.env.PATH,
      path.join(os.homedir(), ".local/bin"),
      "/opt/homebrew/bin",
      "/usr/local/bin",
      "/usr/bin",
      "/bin",
      "/usr/sbin",
      "/sbin",
    ]
      .filter(Boolean)
      .join(":"),
  };

  return new Promise((resolve, reject) => {
    execFile(
      command,
      args,
      { encoding: "utf8", env, maxBuffer: 10 * 1024 * 1024 },
      (error, stdout, stderr) => {
        const result = { stdout, stderr };
        if (error) {
          const code = "code" in error ? (error.code ?? undefined) : undefined;
          const message =
            code === "ENOENT"
              ? `Could not find ssht at ${command}`
              : stderr.trim() || error.message;
          reject(
            new SshtError(message, {
              ...result,
              code,
            }),
          );
          return;
        }
        resolve(result);
      },
    );
  });
}

function configArgs(preferences: Preferences): string[] {
  const args: string[] = [];
  const configPath = normalizeText(preferences.configPath);
  if (configPath) {
    args.push("--config", expandHome(configPath));
  }
  if (preferences.noInclude) {
    args.push("--no-include");
  }
  return args;
}

function groupHosts(hosts: HostEntry[]): Array<[string, HostEntry[]]> {
  const grouped = new Map<string, HostEntry[]>();
  for (const host of hosts) {
    const group = normalizeText(host.group) || "ungrouped";
    grouped.set(group, [...(grouped.get(group) ?? []), host]);
  }
  return Array.from(grouped.entries());
}

function hostKeywords(host: HostEntry): string[] {
  return [
    host.alias,
    host.group,
    host.hostName,
    host.user,
    host.port,
    host.identityFile,
    host.proxyJump,
    host.proxyCommand,
    ...(host.tags ?? []),
  ].filter((value): value is string => Boolean(value));
}

function formatSubtitle(host: HostEntry): string {
  if (host.user && host.hostName) {
    return `${host.user}@${host.hostName}`;
  }
  return host.hostName || host.user || host.sourceFile;
}

function hostMarkdown(host: HostEntry): string {
  const rows: Array<[string, string | undefined]> = [
    ["Alias", host.alias],
    ["Group", host.group],
    ["HostName", host.hostName],
    ["User", host.user],
    ["Port", host.port],
    ["IdentityFile", host.identityFile],
    ["ProxyJump", host.proxyJump],
    ["ProxyCommand", host.proxyCommand],
    ["Tags", host.tags?.join(", ")],
    [
      "Source",
      host.sourceFile ? `${host.sourceFile}:${host.sourceLine}` : undefined,
    ],
  ];

  return [
    `# ${escapeMarkdown(host.alias)}`,
    "",
    "| Field | Value |",
    "| --- | --- |",
    ...rows.map(
      ([name, value]) => `| ${name} | ${value ? inlineCode(value) : "-"} |`,
    ),
    "",
    "```sshconfig",
    host.rawBlock?.trim() || `Host ${host.alias}`,
    "```",
  ].join("\n");
}

function inlineCode(value: string): string {
  return `\`${value.replaceAll("`", "\\`").replaceAll("|", "\\|")}\``;
}

function escapeMarkdown(value: string): string {
  return value
    .replaceAll("\\", "\\\\")
    .replaceAll("*", "\\*")
    .replaceAll("_", "\\_")
    .replaceAll("#", "\\#");
}

function formatError(error: unknown): string {
  if (error instanceof SshtError) {
    return (
      error.message || error.stderr || error.stdout || "Unknown ssht error"
    );
  }
  if (error instanceof Error) {
    return error.message;
  }
  return String(error);
}

function normalizeText(value: string | undefined): string {
  return (value ?? "").trim();
}

function expandHome(value: string): string {
  if (value === "~") {
    return os.homedir();
  }
  if (value.startsWith("~/")) {
    return path.join(os.homedir(), value.slice(2));
  }
  return value;
}

function shellQuote(value: string): string {
  return `'${value.replaceAll("'", "'\\''")}'`;
}
