package fish.nexa.ui.dialogs

/** Every dialog the shell, the lists and the chat open (ModalHost's kinds), by name. */
object Dialogs {
    /** Make a server, or join one with an invite; with a template link, on Import a Template with it in. */
    fun createServer(template: String? = null) = open("createServer") { layer, close -> CreateServerSheet(layer, close, template) }

    /** A link to a server (landing in one of its channels), carrying its key unless told not to. */
    fun invite(serverId: String, channelId: String?) = open("invite") { layer, close -> InviteSheet(layer, serverId, channelId, close) }

    /**
     * An invite link opened in the app (`/invite/<code>`): where it leads, with
     * Join or Open. [hash] is the link's part after `#`, whose key unlocks the
     * history once joined.
     */
    fun openInvite(code: String, hash: String = "") = open("openInvite") { layer, close -> OpenInviteSheet(layer, code, hash, close) }

    fun createChannel(serverId: String, parentId: String?) = open("createChannel") { layer, close -> CreateChannelSheet(layer, serverId, parentId, close) }
    fun createCategory(serverId: String) = open("createCategory") { layer, close -> CreateCategorySheet(layer, serverId, close) }
    fun serverNotifications(serverId: String) = open("serverNotifications") { layer, close -> ServerNotificationsSheet(layer, serverId, close) }

    /** Your nickname in a server, or (with Manage Nicknames) someone else's. */
    fun serverProfile(serverId: String, userId: String? = null) = open("serverProfile") { layer, close -> ServerProfileSheet(layer, serverId, userId, close) }

    /** A group's name, and (for its owner) its icon. */
    fun editGroup(channelId: String) = open("editGroup") { layer, close -> EditGroupSheet(layer, channelId, close) }

    /** Friends added to a group straight from the list, or a link that adds whoever opens it. */
    fun groupInvite(channelId: String) = open("groupInvite") { layer, close -> GroupInviteSheet(layer, channelId, close) }

    fun quickSwitcher() = openQuickSwitcher()
    fun createDm(initial: List<String> = emptyList(), groupId: String? = null) = openCreateDmDialog(initial, groupId)
    fun customStatus() = open("customStatus") { layer, close -> CustomStatusSheet(layer, close) }

    /** A new subchat under [parentId], or the one [channelId] names, edited. */
    fun subchat(parentId: String, channelId: String?) = open("subchat") { layer, close -> SubchatSheet(layer, parentId, channelId, close) }

    /** A channel's whole topic, which its header shows one line of. */
    fun channelTopic(channelId: String) = open("channelTopic") { layer, close -> ChannelTopicSheet(layer, channelId, close) }
}
