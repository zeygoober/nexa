package fish.nexa.ui.server

import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.text.BasicText
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import fish.nexa.Nexa
import fish.nexa.net.wireJson
import fish.nexa.ui.components.ButtonKind
import fish.nexa.ui.components.ButtonSize
import fish.nexa.ui.components.NxButton
import fish.nexa.ui.components.NxTextField
import fish.nexa.ui.components.Txt
import fish.nexa.ui.describeError
import fish.nexa.ui.markdown.AppLinks
import fish.nexa.ui.shell.confirm
import fish.nexa.ui.t
import fish.nexa.ui.theme.Nx
import fish.nexa.ui.theme.Radii
import fish.nexa.ui.theme.sunken
import kotlinx.coroutines.launch
import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put

/** A server's template link (`GET /servers/{id}/template`). */
@Serializable
data class ServerTemplateInfo(
    val code: String,
    val url: String = "",
    val name: String = "",
    val description: String = "",
    @SerialName("usage_count") val usageCount: Int = 0,
    @SerialName("synced_at") val syncedAt: Long = 0,
)

/** What the server allows (maxTemplateDesc, cleanServerName). */
private const val DESC_MAX = 120
private const val NAME_MAX = 100

private fun templateOf(res: kotlinx.serialization.json.JsonElement): ServerTemplateInfo? {
    val el = (res as? JsonObject)?.get("template") ?: return null
    if (el is JsonNull) return null
    return wireJson.decodeFromJsonElement(ServerTemplateInfo.serializer(), el)
}

/**
 * Server Settings, Server Template (web `ServerTemplatePage.tsx`): one link that
 * lets anyone make a new server with this one's channels, roles and
 * permissions, as it was when made or last synced. Laid out as the vanity link
 * is: the address in one well, what it has done under it.
 */
@Composable
fun ServerTemplatePage(serverId: String) {
    val c = Nx.colors
    val type = Nx.type
    val scope = rememberCoroutineScope()
    val servers by Nexa.servers.byId.collectAsState()
    val serverName = servers[serverId]?.server?.name ?: ""
    var loaded by remember { mutableStateOf(false) }
    var tpl by remember { mutableStateOf<ServerTemplateInfo?>(null) }
    var failed by remember { mutableStateOf(false) }
    var attempt by remember { mutableIntStateOf(0) }
    var name by remember { mutableStateOf("") }
    var desc by remember { mutableStateOf("") }
    var busy by remember { mutableStateOf<String?>(null) }

    fun take(next: ServerTemplateInfo?) {
        tpl = next
        name = next?.name ?: serverName
        desc = next?.description ?: ""
    }

    LaunchedEffect(serverId, attempt) {
        try {
            take(templateOf(Nexa.api.get("/servers/$serverId/template")))
            loaded = true
            failed = false
        } catch (e: kotlinx.coroutines.CancellationException) {
            throw e
        } catch (_: Exception) {
            failed = true
        }
    }

    val current = tpl
    val changed = current != null && (name.trim() != current.name || desc.trim() != current.description)
    val canSave = name.isNotBlank() && (current == null || changed)

    fun save() {
        if (busy != null || !canSave) return
        busy = "save"
        val made = current == null
        scope.launch {
            try {
                val res = Nexa.api.request("PUT", "/servers/$serverId/template", buildJsonObject {
                    put("name", name.trim())
                    put("description", desc.trim())
                })
                take(templateOf(res))
                Nexa.ui.toast(t(if (made) "server.template.created" else "server.template.saved"), "success")
            } catch (e: Exception) {
                Nexa.ui.toast(describeError(e, t("server.template.failed")), "error")
            } finally {
                busy = null
            }
        }
    }
    fun sync() {
        if (busy != null) return
        busy = "sync"
        scope.launch {
            try {
                take(templateOf(Nexa.api.request("POST", "/servers/$serverId/template/sync")))
                Nexa.ui.toast(t("server.template.synced"), "success")
            } catch (e: Exception) {
                Nexa.ui.toast(describeError(e, t("server.template.failed")), "error")
            } finally {
                busy = null
            }
        }
    }
    fun remove() {
        if (current == null) return
        confirm(t("server.template.deleteTitle"), t("server.template.deleteBody"), t("server.template.delete")) {
            try {
                Nexa.api.request("DELETE", "/servers/$serverId/template")
                take(null)
                Nexa.ui.toast(t("server.template.deleted"), "success")
            } catch (e: Exception) {
                Nexa.ui.toast(describeError(e, t("server.template.failed")), "error")
            }
        }
    }

    SettingsPage {
        PageTitle(t("server.template.title"), gap = 10.dp)
        Hint(t("server.template.intro"), top = 0.dp, modifier = Modifier.padding(bottom = 20.dp))
        when {
            failed -> LoadFailed(t("server.template.loadFailed")) { attempt++ }
            !loaded -> Loading(t("chat.pins.loading"))
            else -> {
                if (current != null) {
                    val host = AppLinks.shareOrigin(Nexa.api.baseUrl).substringAfter("://")
                    val link = current.url.ifEmpty { "${AppLinks.shareOrigin(Nexa.api.baseUrl)}/template/${current.code}" }
                    Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(8.dp), verticalAlignment = Alignment.CenterVertically) {
                        Row(
                            Modifier
                                .weight(1f)
                                .sunken(Radii.Sm, c)
                                .border(1.dp, c.borderSubtle, Radii.Sm)
                                .height(42.dp)
                                .padding(horizontal = 12.dp),
                            verticalAlignment = Alignment.CenterVertically,
                        ) {
                            // the address gives way first on a narrow screen, from its start
                            BasicText(
                                "$host/template/",
                                style = type.body.copy(fontSize = type.sp(15f), color = c.textMuted, textAlign = androidx.compose.ui.text.style.TextAlign.End),
                                maxLines = 1,
                                overflow = TextOverflow.StartEllipsis,
                                modifier = Modifier.weight(1f, fill = false),
                            )
                            Txt(current.code, style = type.input.copy(fontWeight = FontWeight(600)), color = c.textStrong, maxLines = 1)
                        }
                        NxButton(t("server.invites.copyLink"), { copyAndSay(link, t("server.invites.linkCopied")) }, icon = "Copy")
                    }
                    Column(Modifier.fillMaxWidth().padding(top = 12.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                        val small = type.bodySmall.copy(fontSize = type.sp(13.5f))
                        Txt(t("server.template.used", "count" to current.usageCount), style = small, color = c.textMuted)
                        Txt(t("server.template.syncedAt", "when" to fish.nexa.ui.voice.formatTimestamp(current.syncedAt)), style = small, color = c.textMuted)
                        Hint(t("server.template.syncHint"), top = 0.dp)
                        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                            NxButton(t("server.template.sync"), ::sync, kind = ButtonKind.SECONDARY, size = ButtonSize.SM, icon = "RefreshCw", loading = busy == "sync")
                            NxButton(t("server.template.delete"), ::remove, kind = ButtonKind.GHOST, size = ButtonSize.SM, icon = "Trash2")
                        }
                    }
                    H5(t("server.template.details"))
                }
                Column(Modifier.widthIn(max = 560.dp), verticalArrangement = Arrangement.spacedBy(16.dp)) {
                    NxTextField(
                        name, { name = it.take(NAME_MAX) },
                        label = t("server.template.name"),
                        required = true,
                        capitalize = true,
                    )
                    NxTextField(
                        desc, { desc = it.take(DESC_MAX) },
                        label = t("server.template.description"),
                        hint = t("server.template.descriptionHint"),
                        singleLine = false,
                        minLines = 2,
                        imeAction = ImeAction.Default,
                        capitalize = true,
                    )
                    Row {
                        NxButton(t(if (current != null) "server.template.save" else "server.template.create"), ::save, enabled = canSave, loading = busy == "save")
                    }
                }
            }
        }
    }
}
