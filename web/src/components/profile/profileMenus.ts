import { reportItem } from "@/lib/reporting";
import type { PublicUser, Role } from "@/api/types";
import { about, button, defineMenu } from "@/lib/contextMenus/registry";
import { copyText } from "@/lib/clipboard";
import { copyBotIdItem } from "@/lib/copyId";
import { confirmRemoveFriend, openBlockDialog, unblockUser } from "@/lib/friendship";
import { useRelationships } from "@/stores/relationships";
import { useUI } from "@/stores/ui";
import { isBot } from "@/lib/botCopies";
import { t } from "@/i18n";

export interface ProfileMoreCtx {
  user: PublicUser;
  serverId?: string | undefined;
  /** A popout or a panel, which have a full profile to open. */
  compact: boolean;
  popout: boolean;
  /** Whether they are a friend here (never on a profile seen from outside). */
  friend: boolean;
  viewFull: () => void;
  copyLink: () => void;
  editNickname: () => void;
}

/** The More menu on a profile. */
export const profileMoreMenu = defineMenu<ProfileMoreCtx>("profileMore", [
  [
    button("reportProfileContent", ({ user, serverId }) => reportItem({ kind: "user", id: user.id, ...(serverId ? { serverId } : {}) })),
    button("reportProfileApp", ({ user }) => (user.flags & 64) !== 0 && reportItem({ kind: "application", id: user.id }, "report-app")),
    // people only; a block ends it, as it ends a message
    button("tradeBadgesFromProfile", ({ user }) => !isBot(user) && useRelationships.getState().byUser[user.id]?.type !== "blocked" && { id: "trade", label: t("profile.profileCard.tradeBadges"), onSelect: () => useUI.getState().openModal("trade", { userId: user.id }) }),
    button("viewFullProfile", ({ compact, viewFull }) => compact && { id: "full", label: t("profile.profileCard.viewFullProfile"), onSelect: viewFull }),
    button("copyProfileCardLink", ({ popout, copyLink }) => popout && { id: "link", label: t("profileLink.copy"), onSelect: copyLink }),
  ],
  [
    button("editFriendNickname", ({ user, friend, editNickname }) => friend && { id: "nick", label: useRelationships.getState().byUser[user.id]?.nickname ? t("profile.profileCard.editFriendNickname") : t("profile.profileCard.addFriendNickname"), onSelect: editNickname }),
    // asked first, as the friends list asks: neither is quick to take back
    button("removeFriendFromProfile", ({ user, friend }) => friend && { id: "rm", label: t("profile.profileCard.removeFriend"), danger: true, onSelect: () => confirmRemoveFriend(user) }),
    // one block dialog everywhere, with its choices about their messages
    button("blockFromProfile", ({ user }) =>
      useRelationships.getState().byUser[user.id]?.type === "blocked"
        ? { id: "unblock", label: t("friends.unblock"), onSelect: () => void unblockUser(user.id) }
        : { id: "block", label: t("profile.profileCard.block"), danger: true, onSelect: () => openBlockDialog(user.id) },
    ),
  ],
  [
    button("copyProfileUserId", ({ user }) => ({ id: "copy", label: t("chat.groupMemberMenu.copyUserId"), onSelect: () => void copyText(user.id, t("clipboard.idCopied")) })),
    button("copyProfileBotId", ({ user }) => copyBotIdItem(user.id)),
  ],
]);
about(profileMoreMenu, ["user"], ({ user }) => ({ userId: user.id }));


/** The roles a profile's + button can give someone in a server. */
export const profileAddRoleMenu = defineMenu<{ roles: Role[]; add: (roleId: string) => void }>("profileAddRole", [
  [button("addProfileRole", ({ roles, add }) => roles.map((r) => ({ id: r.id, label: r.name, onSelect: () => add(r.id) })))],
]);
