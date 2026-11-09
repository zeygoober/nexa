package fish.nexa.net

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonPrimitive

/*
 * Apps on the wire (web/src/api/apps.ts): programs with a bot account, made in
 * Settings, Developer Portal, and added to servers and accounts. Ids are
 * Nexa's own; `protocol_id` is the id bot libraries know an app by, for its
 * owner to copy.
 */

/** The privileged events and reads an app is allowed. */
@Serializable
data class AppIntents(
    val presence: Boolean = false,
    val members: Boolean = false,
    /** It reads every message it can see, not only what is addressed to it. */
    @SerialName("message_content") val messageContent: Boolean = false,
)

/** What adding an app asks for by default: its scopes, and its bot's permissions (a decimal string). */
@Serializable
data class InstallParams(val scopes: List<String> = emptyList(), val permissions: String = "0")

/** One way an app can be added ("0" to a server, "1" to an account), with what it asks for that way. */
@Serializable
data class IntegrationTypeConfig(@SerialName("oauth2_install_params") val installParams: InstallParams? = null)

/** An app as its owner sees it in the Developer Portal. */
@Serializable
data class OwnedApp(
    val id: Snowflake,
    val name: String = "",
    val description: String = "",
    @SerialName("icon_id") val iconId: Snowflake? = null,
    @SerialName("cover_id") val coverId: Snowflake? = null,
    val tags: List<String> = emptyList(),
    @SerialName("terms_url") val termsUrl: String = "",
    @SerialName("privacy_url") val privacyUrl: String = "",
    @SerialName("interactions_url") val interactionsUrl: String = "",
    @SerialName("custom_install_url") val customInstallUrl: String = "",
    @SerialName("install_params") val installParams: InstallParams? = null,
    @SerialName("integration_types") val integrationTypes: Map<String, IntegrationTypeConfig> = emptyMap(),
    @SerialName("redirect_uris") val redirectUris: List<String> = emptyList(),
    @SerialName("bot_public") val botPublic: Boolean = true,
    /** Its developer listed it on Discover, which only a public app can be. */
    val discoverable: Boolean = false,
    /** Staff took its listing down. */
    @SerialName("discovery_hidden") val discoveryHidden: Boolean = false,
    @SerialName("bot_require_code_grant") val botRequireCodeGrant: Boolean = false,
    /** The Ed25519 key its interactions endpoint checks requests against, in hex. */
    @SerialName("verify_key") val verifyKey: String = "",
    @SerialName("event_webhooks_url") val eventWebhooksUrl: String = "",
    @SerialName("event_webhooks_enabled") val eventWebhooksEnabled: Boolean = false,
    @SerialName("event_webhooks_types") val eventWebhooksTypes: List<String> = emptyList(),
    val intents: AppIntents = AppIntents(),
    val flags: Long = 0,
    @SerialName("has_secret") val hasSecret: Boolean = false,
    @SerialName("protocol_id") val protocolId: String = "",
    val bot: PublicUser,
    @SerialName("server_count") val serverCount: Int = 0,
    @SerialName("user_install_count") val userInstallCount: Int = 0,
    @SerialName("created_at") val createdAt: Long = 0,
)

/** An app as anyone is shown it. */
@Serializable
data class AppSummary(
    val id: Snowflake,
    val name: String = "",
    val description: String = "",
    @SerialName("icon_id") val iconId: Snowflake? = null,
    val bot: PublicUser? = null,
    @SerialName("terms_url") val termsUrl: String = "",
    @SerialName("privacy_url") val privacyUrl: String = "",
    val intents: AppIntents = AppIntents(),
    @SerialName("bot_public") val botPublic: Boolean = true,
    @SerialName("install_params") val installParams: InstallParams? = null,
    @SerialName("integration_types") val integrationTypes: Map<String, IntegrationTypeConfig> = emptyMap(),
    @SerialName("custom_install_url") val customInstallUrl: String = "",
    @SerialName("owner_id") val ownerId: Snowflake? = null,
    @SerialName("created_at") val createdAt: Long = 0,
    /** On the consent page: the ways it can be added, 0 to a server, 1 to an account. */
    @SerialName("install_types") val installTypes: List<Int> = emptyList(),
)

/** A choice an option offers, or one an app suggests while it is typed; `value` is a string or a number. */
@Serializable
data class CommandChoice(
    val name: String,
    @SerialName("name_localizations") val nameLocalizations: Map<String, String>? = null,
    val value: JsonPrimitive,
)

/** An option of a slash command, or a subcommand, or a group of them (the OPT_ numbers in AppCommands). */
@Serializable
data class CommandOption(
    val type: Int,
    val name: String,
    @SerialName("name_localizations") val nameLocalizations: Map<String, String>? = null,
    val description: String = "",
    @SerialName("description_localizations") val descriptionLocalizations: Map<String, String>? = null,
    val required: Boolean = false,
    val choices: List<CommandChoice>? = null,
    val options: List<CommandOption>? = null,
    @SerialName("channel_types") val channelTypes: List<Int>? = null,
    @SerialName("min_value") val minValue: Double? = null,
    @SerialName("max_value") val maxValue: Double? = null,
    @SerialName("min_length") val minLength: Int? = null,
    @SerialName("max_length") val maxLength: Int? = null,
    val autocomplete: Boolean = false,
)

/** A command an app made: typed after a slash (1), or in a person's (2) or a message's (3) menu. */
@Serializable
data class AppCommand(
    val id: Snowflake,
    @SerialName("application_id") val applicationId: Snowflake,
    @SerialName("guild_id") val guildId: Snowflake? = null,
    val version: Snowflake = "",
    val type: Int = 1,
    val name: String,
    @SerialName("name_localizations") val nameLocalizations: Map<String, String>? = null,
    val description: String = "",
    @SerialName("description_localizations") val descriptionLocalizations: Map<String, String>? = null,
    val options: List<CommandOption>? = null,
    @SerialName("default_member_permissions") val defaultMemberPermissions: String? = null,
    val nsfw: Boolean = false,
    /** In the owner's list, the server a command was made for alone. */
    @SerialName("guild_name") val guildName: String? = null,
)

/** An app whose commands can be used somewhere, and how it is there ("guild", "user"). */
@Serializable
data class CommandApp(
    val id: Snowflake,
    val name: String = "",
    val description: String = "",
    @SerialName("icon_id") val iconId: Snowflake? = null,
    val bot: PublicUser? = null,
    val installed: List<String> = emptyList(),
)

/** What can be used in a channel: the apps there and their commands; `context` 0 a server, 1 a conversation with the app's bot, 2 any other. */
@Serializable
data class ChannelCommands(
    val applications: List<CommandApp> = emptyList(),
    val commands: List<AppCommand> = emptyList(),
    val context: Int = 0,
)

/** A select option of a form's field. */
@Serializable
data class FormOption(
    val label: String,
    val value: String,
    val description: String? = null,
    val emoji: FormEmoji? = null,
    val default: Boolean = false,
)

@Serializable
data class FormEmoji(val id: String? = null, val name: String? = null, val animated: Boolean = false)

@Serializable
data class FormDefault(val id: Snowflake, val type: String)

/** One field of an app's form: 4 text, 3 choices, 5 people, 6 roles, 7 people and roles, 8 channels, 19 files, 21 one of, 22 any of, 23 a tick. */
@Serializable
data class FormField(
    val type: Int,
    val id: Int = 0,
    @SerialName("custom_id") val customId: String = "",
    /** For text: 1 one line, 2 paragraphs. */
    val style: Int? = null,
    val placeholder: String? = null,
    val value: String? = null,
    @SerialName("min_length") val minLength: Int? = null,
    @SerialName("max_length") val maxLength: Int? = null,
    val required: Boolean = true,
    val options: List<FormOption>? = null,
    @SerialName("min_values") val minValues: Int? = null,
    @SerialName("max_values") val maxValues: Int? = null,
    @SerialName("default_values") val defaultValues: List<FormDefault>? = null,
    @SerialName("channel_types") val channelTypes: List<Int>? = null,
    @SerialName("file_types") val fileTypes: List<String>? = null,
    val default: Boolean? = null,
)

/** One line of a form: a field with a label (1 or 18), or text (10). */
@Serializable
data class FormPart(
    val type: Int,
    val id: Int = 0,
    val label: String? = null,
    val description: String? = null,
    val content: String? = null,
    val component: FormField? = null,
)

/** A form an app opened. */
@Serializable
data class AppForm(
    @SerialName("custom_id") val customId: String,
    val title: String = "",
    val components: List<FormPart> = emptyList(),
)

/**
 * An app's first answer to something someone used: 4 a message (in
 * `message`), 5 a message it is still thinking about, 6 nothing yet, 7 its
 * message changed, 8 suggestions for an option, 9 a form to fill in.
 */
@Serializable
data class InteractionAnswer(
    val id: Snowflake = "",
    val type: Int,
    val message: Message? = null,
    val choices: List<CommandChoice>? = null,
    val modal: AppForm? = null,
)

/** A file sent with a command's file option or a form's file field, kept for the app. */
@Serializable
data class InteractionFile(
    val id: Snowflake,
    val filename: String = "",
    val size: Long = 0,
    @SerialName("content_type") val contentType: String = "",
)

/** A server someone can add an app to, on the consent page. */
@Serializable
data class AuthorizeServer(
    val id: Snowflake,
    val name: String = "",
    @SerialName("icon_id") val iconId: Snowflake? = null,
    /** The permissions they hold there, a decimal string. */
    val permissions: String = "0",
    @SerialName("has_bot") val hasBot: Boolean = false,
    val installed: Boolean = false,
    val banned: Boolean = false,
)

/** What a link to add an app asks for, for the consent page. */
@Serializable
data class AuthorizeInfo(
    val application: AppSummary,
    val scopes: List<String> = emptyList(),
    /** The bot's permissions asked for, a decimal string. */
    val permissions: String = "0",
    /** 0 to a server, 1 to the account. */
    @SerialName("integration_type") val integrationType: Int = 0,
    @SerialName("redirect_uri") val redirectUri: String = "",
    @SerialName("guild_id") val guildId: Snowflake? = null,
    @SerialName("disable_guild_select") val disableGuildSelect: Boolean = false,
    /** The link said how the app is added; otherwise, where it can go both ways, the person chooses. */
    @SerialName("type_chosen") val typeChosen: Boolean = false,
    val servers: List<AuthorizeServer>? = null,
    val installed: Boolean = false,
)

/** An app someone let in on their account; `installed` when it is added to the account, so its commands work everywhere. */
@Serializable
data class AuthorizedApp(
    val application: AppSummary,
    val scopes: List<String> = emptyList(),
    @SerialName("authorized_at") val authorizedAt: Long = 0,
    val installed: Boolean = false,
)

/** An app added to a server, for its managers. */
@Serializable
data class ServerApp(
    val application: AppSummary,
    val scopes: List<String> = emptyList(),
    @SerialName("installed_by") val installedBy: Snowflake? = null,
    @SerialName("installed_at") val installedAt: Long = 0,
    /** Its bot is a member. */
    @SerialName("bot_member") val botMember: Boolean = false,
    /** The role made for its bot. */
    @SerialName("role_id") val roleId: Snowflake? = null,
    @SerialName("command_count") val commandCount: Int = 0,
)

/** Who may use a command, or all of an app's commands: a role (1), a person (2) or a channel (3), allowed or not. */
@Serializable
data class CommandPermission(val id: Snowflake, val type: Int, val permission: Boolean)

/** An app's commands in a server, with who may use each: by command id, the app's own id holding what applies to all of them. */
@Serializable
data class IntegrationCommands(
    val commands: List<AppCommand> = emptyList(),
    val permissions: Map<Snowflake, List<CommandPermission>> = emptyMap(),
    /** The id that stands for every channel in a channel entry. */
    @SerialName("all_channels_id") val allChannelsId: Snowflake = "",
)

/** What someone used, as their app sends it: 2 a command, 3 a button or select menu, 4 suggestions for an option, 5 a form. */
@Serializable
data class InteractRequest(
    val type: Int,
    @SerialName("application_id") val applicationId: Snowflake,
    @SerialName("channel_id") val channelId: Snowflake,
    @SerialName("message_id") val messageId: Snowflake? = null,
    /** For a form: the interaction that opened it. */
    @SerialName("modal_of") val modalOf: Snowflake? = null,
    val data: JsonElement,
    val locale: String? = null,
    /** For a message command on a message the app cannot read: that message, sealed to the app's bot (base64). */
    val copy: String? = null,
)
