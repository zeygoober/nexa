import { api } from "./client";
import type { Message, PublicUser, Snowflake } from "./types";

/**
 * Apps: programs with a bot account, made in Settings, Developer Portal, and
 * added to servers and accounts. Bot libraries talk to the server through the
 * bot API (internal/api/bot_*.go); this is the app's side of it: making and
 * changing apps, adding them, managing them in a server, and using what they
 * made (commands, buttons, select menus and forms).
 *
 * Ids here are Nexa's own; `protocol_id` is the id bot libraries know an app
 * by, for its owner to copy.
 */

/** The privileged events and reads an app is allowed. */
export interface AppIntents {
  presence: boolean;
  members: boolean;
  /** It reads every message it can see, not only what is addressed to it. */
  message_content: boolean;
}

/** What adding an app asks for by default: its scopes, and its bot's permissions (a decimal string). */
export interface InstallParams {
  scopes: string[];
  permissions: string;
}

/** How an app can be added: "0" to a server, "1" to an account. */
export type IntegrationTypes = Partial<Record<"0" | "1", { oauth2_install_params?: InstallParams }>>;

/** An app as its owner sees it in the Developer Portal. */
export interface OwnedApp {
  id: Snowflake;
  name: string;
  description: string;
  icon_id: Snowflake | null;
  cover_id: Snowflake | null;
  tags: string[];
  terms_url: string;
  privacy_url: string;
  interactions_url: string;
  custom_install_url: string;
  install_params: InstallParams | null;
  integration_types: IntegrationTypes;
  redirect_uris: string[];
  bot_public: boolean;
  /** On Discover, by its developer's choice (and public, and not taken off by staff). */
  discoverable: boolean;
  discovery_hidden: boolean;
  bot_require_code_grant: boolean;
  /** The Ed25519 key its interactions endpoint checks requests against, in hex. */
  verify_key: string;
  event_webhooks_url: string;
  event_webhooks_enabled: boolean;
  event_webhooks_types: string[];
  intents: AppIntents;
  flags: number;
  has_secret: boolean;
  protocol_id: string;
  bot: PublicUser;
  server_count: number;
  user_install_count: number;
  created_at: number;
}

/** An app as anyone is shown it. */
export interface AppSummary {
  id: Snowflake;
  name: string;
  description: string;
  icon_id: Snowflake | null;
  bot: PublicUser;
  terms_url?: string;
  privacy_url?: string;
  intents?: AppIntents;
  bot_public?: boolean;
  install_params?: InstallParams | null;
  integration_types?: IntegrationTypes;
  custom_install_url?: string;
  owner_id?: Snowflake;
  created_at?: number;
}

/** What changes on an app; `icon` and `cover_image` are image data, or null to take them away. */
export interface AppPatch {
  name?: string;
  description?: string;
  tags?: string[];
  terms_url?: string;
  privacy_url?: string;
  interactions_url?: string;
  custom_install_url?: string;
  install_params?: InstallParams | null;
  integration_types?: IntegrationTypes;
  redirect_uris?: string[];
  bot_public?: boolean;
  discoverable?: boolean;
  bot_require_code_grant?: boolean;
  event_webhooks_url?: string;
  event_webhooks_enabled?: boolean;
  event_webhooks_types?: string[];
  intents?: Partial<AppIntents>;
  icon?: string | null;
  cover_image?: string | null;
}

/** A choice an option offers, or one an app suggests while it is typed. */
export interface CommandChoice {
  name: string;
  name_localizations?: Record<string, string>;
  value: string | number;
}

/** An option of a slash command, or a subcommand, or a group of them. */
export interface CommandOption {
  /** 1 subcommand, 2 group, 3 text, 4 whole number, 5 yes or no, 6 person, 7 channel, 8 role, 9 person or role, 10 number, 11 file. */
  type: number;
  name: string;
  name_localizations?: Record<string, string>;
  description: string;
  description_localizations?: Record<string, string>;
  required?: boolean;
  choices?: CommandChoice[];
  options?: CommandOption[];
  channel_types?: number[];
  min_value?: number;
  max_value?: number;
  min_length?: number;
  max_length?: number;
  autocomplete?: boolean;
}

/** A command an app made: typed after a slash (1), or in a person's (2) or a message's (3) menu. */
export interface AppCommand {
  id: Snowflake;
  application_id: Snowflake;
  guild_id?: Snowflake;
  version: Snowflake;
  type: number;
  name: string;
  name_localizations?: Record<string, string>;
  description: string;
  description_localizations?: Record<string, string>;
  options?: CommandOption[];
  default_member_permissions: string | null;
  nsfw?: boolean;
  integration_types?: number[];
  contexts?: number[] | null;
  /** In the owner's list, the server a command was made for alone. */
  guild_name?: string;
}

/** An app whose commands can be used somewhere, and how it is there. */
export interface CommandApp {
  id: Snowflake;
  name: string;
  description: string;
  icon_id: Snowflake | null;
  bot: PublicUser;
  installed: ("guild" | "user")[];
}

/** What can be used in a channel: the apps there and their commands. */
export interface ChannelCommands {
  applications: CommandApp[];
  commands: AppCommand[];
  /** 0 a server, 1 a conversation with the app's bot, 2 any other conversation. */
  context: number;
}

/** One field of an app's form. */
export interface FormField {
  /** 4 text, 3 choices, 5 people, 6 roles, 7 people and roles, 8 channels, 19 files, 21 one of, 22 any of, 23 a tick. */
  type: number;
  id: number;
  custom_id: string;
  /** For text: 1 one line, 2 paragraphs. */
  style?: number;
  placeholder?: string;
  value?: string;
  min_length?: number;
  max_length?: number;
  required: boolean;
  options?: { label: string; value: string; description?: string; emoji?: { id?: string; name?: string; animated?: boolean }; default?: boolean }[];
  min_values?: number;
  max_values?: number;
  default_values?: { id: Snowflake; type: "user" | "role" | "channel" }[];
  channel_types?: number[];
  file_types?: string[];
  default?: boolean;
}

/** One line of a form: a field with a label (1 or 18), or text (10). */
export interface FormPart {
  type: number;
  id: number;
  label?: string;
  description?: string;
  content?: string;
  component?: FormField;
}

/** A form an app opened. */
export interface AppForm {
  custom_id: string;
  title: string;
  components: FormPart[];
}

/**
 * An app's first answer to something someone used: 4 a message (in
 * `message`), 5 a message it is still thinking about, 6 nothing yet, 7 its
 * message changed, 8 suggestions for an option, 9 a form to fill in.
 */
export interface InteractionAnswer {
  id: Snowflake;
  type: number;
  message?: Message;
  choices?: CommandChoice[];
  modal?: AppForm;
}

/** What someone used, as their app sends it. */
export interface InteractRequest {
  /** 2 a command, 3 a button or select menu, 4 suggestions for an option, 5 a form. */
  type: number;
  application_id: Snowflake;
  channel_id: Snowflake;
  message_id?: Snowflake;
  /** For a form: the interaction that opened it. */
  modal_of?: Snowflake;
  data: unknown;
  locale?: string;
  /** For a message command on a message the app cannot read: that message, sealed to the app's bot (base64). */
  copy?: string;
}

/** A file sent with a command's file option or a form's file field, kept for the app. */
export interface InteractionFile {
  id: Snowflake;
  filename: string;
  size: number;
  content_type: string;
}

/** A server someone can add an app to, on the consent page. */
export interface AuthorizeServer {
  id: Snowflake;
  name: string;
  icon_id: Snowflake | null;
  /** The permissions they hold there, a decimal string. */
  permissions: string;
  has_bot: boolean;
  installed: boolean;
  banned: boolean;
}

/** What a link to add an app asks for, for the consent page. */
export interface AuthorizeInfo {
  /** `install_types` are the ways it can be added: 0 to a server, 1 to an account. */
  application: AppSummary & { intents: AppIntents; install_types: number[] };
  scopes: string[];
  /** The bot's permissions asked for, a decimal string. */
  permissions: string;
  /** 0 to a server, 1 to the account. */
  integration_type: number;
  redirect_uri: string;
  guild_id: Snowflake | null;
  disable_guild_select: boolean;
  /** The link said how the app is added; otherwise, where it can go both ways, the person chooses. */
  type_chosen: boolean;
  servers?: AuthorizeServer[];
  installed?: boolean;
}

/** An app someone let in on their account. */
export interface AuthorizedApp {
  application: AppSummary;
  scopes: string[];
  authorized_at: number;
  /** It is added to the account, so its commands work everywhere. */
  installed: boolean;
}

/** An app added to a server, for its managers. */
export interface ServerApp {
  application: AppSummary & { intents: AppIntents };
  scopes: string[];
  installed_by: Snowflake;
  installed_at: number;
  /** Its bot is a member. */
  bot_member?: boolean;
  /** The role made for its bot. */
  role_id?: Snowflake;
  command_count?: number;
}

/** Who may use a command, or all of an app's commands: a role (1), a person (2) or a channel (3), allowed or not. */
export interface CommandPermission {
  id: Snowflake;
  type: 1 | 2 | 3;
  permission: boolean;
}

/** An app's commands in a server, with who may use each. */
export interface IntegrationCommands {
  commands: AppCommand[];
  /** By command id; the app's own id holds what applies to all of them. */
  permissions: Record<Snowflake, CommandPermission[]>;
  /** The id that stands for every channel in a channel entry. */
  all_channels_id: Snowflake;
}

export const apps = {
  // the Developer Portal
  mine: () => api.request<OwnedApp[]>("GET", "/applications"),
  create: (name: string) => api.request<{ application: OwnedApp; token: string }>("POST", "/applications", { name }),
  get: (id: Snowflake) => api.request<OwnedApp>("GET", `/applications/${id}`),
  update: (id: Snowflake, patch: AppPatch) => api.request<OwnedApp>("PATCH", `/applications/${id}`, patch),
  updateBot: (id: Snowflake, patch: { username?: string; avatar?: string | null; banner?: string | null }) => api.request<PublicUser>("PATCH", `/applications/${id}/bot`, patch),
  resetToken: (id: Snowflake, authKey: string) => api.request<{ token: string }>("POST", `/applications/${id}/bot/reset`, { auth_key: authKey }),
  resetSecret: (id: Snowflake, authKey: string) => api.request<{ secret: string }>("POST", `/applications/${id}/secret/reset`, { auth_key: authKey }),
  remove: (id: Snowflake, authKey: string) => api.request<void>("POST", `/applications/${id}/delete`, { auth_key: authKey }),
  /** An app's commands everywhere (not one server's own), for its owner. */
  commands: (id: Snowflake) => api.request<AppCommand[]>("GET", `/applications/${id}/commands`),
  /** Takes one of an app's commands away, for its owner. */
  deleteCommand: (id: Snowflake, commandId: Snowflake) => api.request<void>("DELETE", `/applications/${id}/commands/${commandId}`),
  /** Takes every command an app made away, single servers' included, for its owner. */
  deleteAllCommands: (id: Snowflake) => api.request<void>("DELETE", `/applications/${id}/commands`),
  /** What anyone is shown of an app. */
  about: (id: Snowflake) => api.request<AppSummary>("GET", `/applications/${id}/public`),

  // using what apps made
  inChannel: (channelId: Snowflake) => api.request<ChannelCommands>("GET", `/channels/${channelId}/commands`),
  interact: (req: InteractRequest) => api.request<InteractionAnswer>("POST", "/interactions", req),
  sendFile: (file: File) => {
    const form = new FormData();
    form.append("file", file, file.name);
    return api.request<InteractionFile>("POST", "/interactions/files", undefined, { raw: form });
  },
  dismiss: (messageId: Snowflake) => api.request<void>("DELETE", `/ephemeral/${messageId}`),

  // adding apps, and the ones let in
  authorizeInfo: (query: string) => api.request<AuthorizeInfo>("GET", `/oauth2/authorize?${query}`),
  authorize: (query: string, opts: { guild_id?: Snowflake; channel_id?: Snowflake; permissions?: string }) =>
    api.request<{ location?: string; guild_id?: Snowflake }>("POST", "/oauth2/authorize", { query, ...opts }),
  authorized: () => api.request<AuthorizedApp[]>("GET", "/oauth2/authorized"),
  deauthorize: (appId: Snowflake) => api.request<void>("DELETE", `/oauth2/authorized/${appId}`),

  // a server's apps, for its managers
  inServer: (serverId: Snowflake) => api.request<ServerApp[]>("GET", `/servers/${serverId}/integrations`),
  removeFromServer: (serverId: Snowflake, appId: Snowflake) => api.request<void>("DELETE", `/servers/${serverId}/integrations/${appId}`),
  serverCommands: (serverId: Snowflake, appId: Snowflake) => api.request<IntegrationCommands>("GET", `/servers/${serverId}/integrations/${appId}/commands`),
  setCommandPermissions: (serverId: Snowflake, appId: Snowflake, commandId: Snowflake, permissions: CommandPermission[]) =>
    api.request<{ command_id: Snowflake; permissions: CommandPermission[] }>("PUT", `/servers/${serverId}/integrations/${appId}/commands/${commandId}/permissions`, { permissions }),
};
