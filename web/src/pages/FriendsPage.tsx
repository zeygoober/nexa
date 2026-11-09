import { personText } from "@/lib/personText";
import { Check, Copy, MessageSquare, MessageSquarePlus, MoreVertical, Phone, Plus, Search, UserPlus, Users, X } from "lucide-react";
import { useEffect, useId, useMemo, useState, type FormEvent } from "react";
import { useNavigate } from "react-router-dom";
import { channels as channelsApi } from "@/api/channels";
import { relationships as relApi } from "@/api/relationships";
import { acceptFriendRequest, addRequestNote, dropFriendRequest, requestError, unblockUser, useBusyWith } from "@/lib/friendship";
import { REQUEST_NOTE_MAX, cleanRequestNote } from "@/lib/requestNote";
import type { Relationship } from "@/api/types";
import { PersonName } from "@/components/common/PersonName";
import { Avatar, statusLabel } from "@/components/common/Avatar";
import { StaffMark } from "@/components/common/OwnerBadge";
import { rankOf } from "@/lib/staffRank";
import { StatusLine } from "@/components/activity/StatusLine";
import { Button } from "@/components/common/Button";
import { Field, TextArea } from "@/components/common/TextInput";
import { Tooltip } from "@/components/common/Tooltip";
import { RequestNote } from "@/components/friends/RequestNote";
import { audioContext } from "@/voice/media";
import { joinVoice } from "@/voice/session";
import { PageHeaderButtons } from "@/components/shell/PageHeader";
import { useChannels } from "@/stores/channels";
import { activitiesOf, customStatus, statusOf, usePresence } from "@/stores/presence";
import { useRelationships } from "@/stores/relationships";
import { useUI } from "@/stores/ui";
import { copyAndSay } from "@/lib/copyText";
import { useUsers } from "@/stores/users";
import { useAuth } from "@/stores/auth";
import { UserTag } from "@/components/servertag/UserTag";
import s from "@/components/friends/Friends.module.css";
import { ReportBugButton, SuggestIconButton } from "@/components/bugs/ReportBugButton";
import bugStyles from "@/components/bugs/Bugs.module.css";
import { guarded } from "@/lib/captcha";
import { describeError, dmOpenError } from "@/lib/errors";
import { PickBar, PickMark } from "@/components/common/Pick";
import { friendsBulk } from "@/components/friends/friendsBulk";
import { friendMenu } from "@/components/friends/friendMenu";
import { pickClick, usePick } from "@/stores/pick";

import { t as tr_, translator } from "@/i18n";
type Tab = "online" | "all" | "pending" | "blocked" | "add";

const NO_PICKS: string[] = [];

/** Online first, then idle, then busy: who can answer soonest heads the list. */
const PRESENCE_ORDER: Record<string, number> = { online: 0, idle: 1, dnd: 2 };

/**
 * The order a tab lists people in. It used to be the order the friendships
 * were made — oldest first — which moved everyone as people were added and
 * made the list a history rather than a directory. By name, in the reader's
 * language; the Online tab by who is most available first; Pending with the
 * requests to answer before the ones waiting on someone else.
 */
export function sortRelationships(list: Relationship[], tab: Tab, presenceOf: (id: string) => string, locale: string): Relationship[] {
  let collator: Intl.Collator;
  try {
    collator = new Intl.Collator(locale, { sensitivity: "base", numeric: true });
  } catch {
    collator = new Intl.Collator(undefined, { sensitivity: "base", numeric: true });
  }
  const byName = (a: Relationship, b: Relationship) => collator.compare(a.user.display_name, b.user.display_name) || collator.compare(a.user.username, b.user.username);
  return [...list].sort((a, b) => {
    if (tab === "online") {
      const d = (PRESENCE_ORDER[presenceOf(a.user.id)] ?? 3) - (PRESENCE_ORDER[presenceOf(b.user.id)] ?? 3);
      if (d) return d;
    }
    if (tab === "pending" && a.type !== b.type) return a.type === "incoming" ? -1 : 1;
    return byName(a, b);
  });
}

export default function FriendsPage() {
  const [tab, setTab] = useState<Tab>("online");
  const [query, setQuery] = useState("");
  const rels = useRelationships((r) => r.byUser);
  const presence = usePresence((p) => p.byUser);
  const users = useUsers((u) => u.byId);
  const list = useMemo(() => {
    const all = Object.values(rels);
    const q = query.trim().toLowerCase().replace(/^@/, "");
    const match = (r: Relationship) => !q || r.user.display_name.toLowerCase().includes(q) || r.user.username.includes(q);
    const shown =
      tab === "online"
        ? all.filter((r) => r.type === "friend" && statusOf(presence[r.user.id]) !== "offline" && match(r))
        : tab === "all"
          ? all.filter((r) => r.type === "friend" && match(r))
          : tab === "pending"
            ? all.filter((r) => (r.type === "incoming" || r.type === "outgoing") && match(r))
            : tab === "blocked"
              ? all.filter((r) => r.type === "blocked" && match(r))
              : [];
    return sortRelationships(shown, tab, (id) => statusOf(presence[id]), translator().locale);
  }, [rels, tab, query, presence]);
  // Nobody at all yet — no friend, no request either way: the first thing a
  // new account sees, which used to be "Nobody is online right now".
  const alone = Object.keys(rels).length === 0;
  const pendingCount = Object.values(rels).filter((r) => r.type === "incoming").length;

  // The count used to be glued onto the label in English and then stripped off
  // again below with an English-shaped regex; it travels beside the label now,
  // so neither half has to survive a round trip through the other's grammar.
  const tabs: { id: Tab; label: string; count?: number }[] = [
    { id: "online", label: tr_("shell.user.online") },
    { id: "all", label: tr_("friends.all") },
    { id: "pending", label: tr_("friends.pending"), ...(pendingCount ? { count: pendingCount } : {}) },
    { id: "blocked", label: tr_("friends.blocked") },
  ];
  const activeTab = tabs.find((x) => x.id === tab);

  // picking several people at once (stores/pick.ts), within the tab on show
  const picked = usePick((p) => (p.list === "friends" ? p.ids : NO_PICKS));
  const pickMode = usePick((p) => p.list === "friends" && p.mode);
  const isPicking = pickMode || picked.length > 0;
  const pickOrder = useMemo(() => list.map((r) => r.user.id), [list]);
  const openModal = useUI((u) => u.openModal);
  // another tab is another list
  useEffect(() => {
    if (usePick.getState().list === "friends") usePick.getState().clear();
  }, [tab]);
  // whoever is picked and has gone from the list (removed, answered) is let go of
  useEffect(() => {
    const gone = picked.filter((id) => !pickOrder.includes(id));
    if (gone.length) usePick.getState().drop(gone);
  }, [picked, pickOrder]);
  const bulk = isPicking ? friendsBulk(picked, openModal) : null;

  // Left/Right across the strip, as a tablist is expected to move; the buttons
  // themselves carry tabIndex -1 so Tab passes over the set rather than through it.
  const order: Tab[] = [...tabs.map((x) => x.id), "add"];
  const onTabKey = (e: React.KeyboardEvent<HTMLButtonElement>) => {
    const step = e.key === "ArrowRight" ? 1 : e.key === "ArrowLeft" ? -1 : e.key === "Home" ? -order.length : e.key === "End" ? order.length : 0;
    if (!step) return;
    e.preventDefault();
    const at = order.indexOf(tab);
    const next = order[Math.min(order.length - 1, Math.max(0, at + step))]!;
    setTab(next);
    const strip = e.currentTarget.parentElement;
    const buttons = strip ? [...strip.querySelectorAll<HTMLElement>('[role="tab"]')] : [];
    buttons[order.indexOf(next)]?.focus();
  };

  return (
    <div className={s.page}>
      <header className={s.header}>
        <PageHeaderButtons />
        <div className={s.title}><Users size={22} /> <span className={s.titleText}>{tr_("sidebar.dMSidebar.friends")}</span></div>
        <div className={bugStyles.friendsActions}>
          <SuggestIconButton from="friends" className={bugStyles.friendsBug} />
          <ReportBugButton from="friends" className={bugStyles.friendsBug} />
        </div>
        <div className={s.tabs} role="tablist" aria-label={tr_("sidebar.dMSidebar.friends")}>
          {tabs.map((x) => (
            <button key={x.id} className={s.tab} role="tab" aria-selected={tab === x.id} tabIndex={tab === x.id ? 0 : -1} onKeyDown={onTabKey} data-selected={tab === x.id} onClick={() => setTab(x.id)}>
              {x.count ? `${x.label} (${x.count})` : x.label}
            </button>
          ))}
          <button className={s.addTab} role="tab" aria-selected={tab === "add"} tabIndex={tab === "add" ? 0 : -1} onKeyDown={onTabKey} data-selected={tab === "add"} onClick={() => setTab("add")}>{tr_("profile.profileCard.addFriend")}</button>
        </div>
      </header>
      <div className={s.body}>
        {tab === "add" ? (
          <AddFriend />
        ) : (
          <>
            <div className={s.search}>
              <input value={query} onChange={(e) => setQuery(e.target.value)} placeholder={tr_("chat.chat.search")} aria-label={tr_("friends.searchFriends")} />
              <Search size={20} />
            </div>
            <div className={s.sectionRow}>
              <h3 className={s.section}>{activeTab?.label} (<span role="status">{list.length}</span>)</h3>
              {list.length > 0 && !isPicking ? (
                <button type="button" className={s.selectBtn} onClick={() => usePick.getState().start("friends")} data-testid="friends-select">{tr_("pick.select")}</button>
              ) : null}
            </div>
            {list.length === 0 ? (
              alone && !query.trim() && (tab === "online" || tab === "all") ? <FirstRun onAdd={() => setTab("add")} /> : <Empty tab={tab} query={query} onClear={() => setQuery("")} />
            ) : (
              list.map((r) => <FriendRow key={r.user.id} rel={r} presenceUser={users[r.user.id] ?? r.user} order={pickOrder} picking={isPicking} picked={picked.includes(r.user.id)} />)
            )}
          </>
        )}
      </div>
      {bulk && tab !== "add" ? <PickBar list="friends" count={picked.length} label={tr_("pick.friendsBar")} actions={bulk.bar} more={bulk.menu} /> : null}
    </div>
  );
}

/**
 * Nothing to show. The list is filtered by the tab *and* by the search box, so
 * a typo in the box used to be answered with "You have no friends yet" — which
 * is a statement about the account, not about the search.
 */
function Empty({ tab, query, onClear }: { tab: Tab; query: string; onClear: () => void }) {
  const text: Record<Tab, string> = {
    online: tr_("friends.nobodyIsOnlineRightNow"),
    all: tr_("friends.noFriendsYet"),
    pending: tr_("friends.thereAreNoPendingFriendRequests"),
    blocked: tr_("friends.youHaveNotBlockedAnyone"),
    add: "",
  };
  const searching = query.trim().length > 0;
  return (
    <div className={s.empty}>
      <Users size={48} />
      <p>{searching ? tr_("friends.nobodyHereMatches", { query: query.trim() }) : text[tab]}</p>
      {searching ? <Button size="sm" variant="secondary" onClick={onClear}>{tr_("friends.clearTheSearch")}</Button> : null}
    </div>
  );
}

/**
 * The first screen of a new account. It said "Nobody is online right now",
 * which is about friends the account does not have, and offered nothing to do:
 * the two ways in — a friend, or a server — are offered here, with the
 * account's own username to send to someone.
 */
function FirstRun({ onAdd }: { onAdd: () => void }) {
  const openModal = useUI((u) => u.openModal);
  return (
    <div className={s.firstRun} data-testid="friends-first-run">
      <div className={s.firstIcon} aria-hidden="true"><Users size={32} /></div>
      <h2 className={s.firstTitle}>{tr_("friends.firstRunTitle")}</h2>
      <p className={s.firstBody}>{tr_("friends.firstRunBody")}</p>
      <div className={s.firstActions}>
        <Button onClick={onAdd}><UserPlus size={16} /> {tr_("profile.profileCard.addFriend")}</Button>
        <Button variant="secondary" onClick={() => openModal("createServer")}><Plus size={16} /> {tr_("friends.createAServer")}</Button>
      </div>
      <YourUsername />
    </div>
  );
}

/** The account's own username, which is what someone needs to add it. */
function YourUsername() {
  const username = useAuth((a) => a.me?.username);
  if (!username) return null;
  return (
    <p className={s.yourName}>
      <span>{tr_("friends.yourUsername", { username: `@${username}` })}</span>
      <Button size="sm" variant="ghost" onClick={() => copyAndSay(username)} aria-label={tr_("friends.copyYourUsername")}><Copy size={14} /></Button>
    </p>
  );
}

function FriendRow({ rel, presenceUser, order, picking, picked }: { rel: Relationship; presenceUser: Relationship["user"]; order: string[]; picking: boolean; picked: boolean }) {
  const nav = useNavigate();
  const p = usePresence((st) => st.byUser[rel.user.id]);
  const toast = useUI((u) => u.toast);
  const upsertChannel = useChannels((c) => c.upsert);
  const busy = useBusyWith(rel.user.id);
  const status = statusOf(p);
  const cs = customStatus(p);
  const acts = activitiesOf(p);
  const name = presenceUser.display_name;
  const profile = () => useUI.getState().openModal("profile", { userId: rel.user.id });

  /** Opens (or makes) the DM with them; returns its id, or null when that failed. */
  const openDM = async (): Promise<string | null> => {
    try {
      const ch = await channelsApi.createDM([rel.user.id]);
      upsertChannel(ch);
      nav(`/channels/@me/${ch.id}`);
      return ch.id;
    } catch (err) {
      toast(dmOpenError(err, tr_("profile.profileCard.couldNotOpenAConversation")), "error");
      return null;
    }
  };
  const message = async () => {
    await openDM();
  };
  // straight into a call with them, ringing them the way the call button in the DM does
  const call = async () => {
    // audio has to start inside the tap (iOS), before waiting on the conversation
    audioContext();
    const id = await openDM();
    if (!id) return;
    try {
      await joinVoice(id);
    } catch (err) {
      // A refused microphone rejects here, and this page said nothing at all:
      // the conversation opened, no call started, and no reason was given.
      toast(describeError(err, tr_("friends.thatCallCouldNotBeStarted")), "error");
    }
  };
  const accept = async () => {
    const ch = await acceptFriendRequest(rel.user);
    // the server opens the conversation as part of accepting; go straight to it
    if (ch) nav(`/channels/@me/${ch.id}`);
  };
  const more = (e: React.MouseEvent) => {
    e.preventDefault();
    const pick = usePick.getState();
    // a right click on one of several picked acts on all of them
    if (pick.list === "friends" && pick.ids.includes(rel.user.id) && pick.ids.length > 1) {
      friendsBulk(pick.ids, useUI.getState().openModal).menu({ x: e.clientX, y: e.clientY, label: tr_("pick.count", { count: pick.ids.length }) });
      return;
    }
    if (pick.list === "friends" && !pick.mode && pick.ids.length) pick.clear();
    // Opened from the keyboard there is no pointer, and the click lands at
    // 0,0: the menu goes by the button instead.
    const fromKeys = e.clientX === 0 && e.clientY === 0;
    const r = (e.currentTarget as HTMLElement).getBoundingClientRect();
    friendMenu.open(
      { rel, profile, message: () => void message(), call: () => void call() },
      { x: fromKeys ? (document.documentElement.dir === "rtl" ? r.right : r.left) : e.clientX, y: fromKeys ? r.bottom + 4 : e.clientY },
    );
  };

  // A friend's line is their status or what they are doing, which scrolls when
  // it is long, as it does in the sidebar; anyone else's says where things stand.
  const sub =
    rel.type === "friend" && (cs || acts[0]) ? (
      <StatusLine className={s.subStatus} status={cs} activity={acts[0]} more={Math.max(0, acts.length - 1)} />
    ) : (
      <span className={s.subText}>{rel.type === "incoming" ? tr_("friends.incomingFriendRequest") : rel.type === "outgoing" ? tr_("friends.outgoingFriendRequest") : rel.type === "blocked" ? tr_("friends.blocked") : statusLabel(status)}</span>
    );

  // The whole row opens their profile: the name and picture are one button,
  // and its hit area is stretched over the row (the space around the action
  // buttons included), while the buttons sit above it and keep their own
  // jobs. Siblings rather than buttons inside a button, as the DM list does.
  return (
    <div className={s.row} data-user={rel.user.username} data-kind={rel.type} onContextMenu={more} data-picked={picked || undefined}>
      <button
        type="button"
        className={s.rowMain}
        onClick={(e) => {
          // Ctrl or ⌘ picks someone, Shift a run of people, a tap one while picking
          if (pickClick(e, "friends", rel.user.id, order)) return;
          profile();
        }}
        aria-haspopup={picking ? undefined : "dialog"}
        aria-pressed={picking ? picked : undefined}
        data-testid="friend-row"
      >
        {picking ? <PickMark on={picked} label={tr_("pick.pickNamed", { name })} /> : null}
        <Avatar id={rel.user.id} avatarId={presenceUser.avatar_id} name={name} size={32} status={rel.type === "friend" ? status : null} />
        <span className={s.rowText}>
          <span className={s.rowName}>
            <PersonName user={presenceUser} className={s.rowDisplay} dir="auto">{name}</PersonName>
            {/* Nexa's staff carry their rank's mark here as in every list */}
            {rankOf(rel.user.id, rel.user.flags) ? (
              <span className={s.rowBadge}><StaffMark rank={rankOf(rel.user.id, rel.user.flags)!} size={14} /></span>
            ) : null}
            <UserTag userId={rel.user.id} size="sm" />
            <span className={s.rowUser}>{presenceUser.username}</span>
          </span>
          {/* On a touch screen the username is never hovered into view, and it
              is what tells two people with the same display name apart. */}
          <span className={s.rowSub}><span className={s.subUser}>{presenceUser.username} · </span>{sub}</span>
          {rel.type === "incoming" && rel.note ? <RequestNote sealed={rel.note} /> : null}
        </span>
      </button>
      <div className={s.actions}>
        {rel.type === "friend" ? (
          <>
            <Tooltip text={tr_("profile.profileCard.message")}><button className={s.act} aria-label={tr_("profile.profileCard.message")} onClick={() => void message()} data-testid="friend-message"><MessageSquare size={20} /></button></Tooltip>
            <Tooltip text={tr_("chat.chat.startVoiceCall")}><button className={s.act} aria-label={tr_("chat.chat.startVoiceCall")} onClick={() => void call()} data-testid="friend-call"><Phone size={20} /></button></Tooltip>
            <Tooltip text={tr_("chat.chat.more")}><button className={s.act} aria-label={tr_("chat.chat.more")} onClick={more} data-testid="friend-more"><MoreVertical size={20} /></button></Tooltip>
          </>
        ) : rel.type === "incoming" ? (
          <>
            <Tooltip text={tr_("profile.profileCard.acceptFriendRequest")}><button className={[s.act, s.ok].join(" ")} aria-label={tr_("profile.profileCard.acceptFriendRequest")} disabled={busy} onClick={() => void accept()} data-testid="friend-accept"><Check size={20} /></button></Tooltip>
            <Tooltip text={tr_("people.menu.ignoreRequest")}><button className={[s.act, s.no].join(" ")} aria-label={tr_("people.menu.ignoreRequest")} disabled={busy} onClick={() => void dropFriendRequest(rel.user.id)} data-testid="friend-ignore"><X size={20} /></button></Tooltip>
          </>
        ) : rel.type === "outgoing" ? (
          <Tooltip text={tr_("people.menu.cancelRequest")}><button className={[s.act, s.no].join(" ")} aria-label={tr_("people.menu.cancelRequest")} disabled={busy} onClick={() => void dropFriendRequest(rel.user.id)} data-testid="friend-cancel"><X size={20} /></button></Tooltip>
        ) : (
          <Tooltip text={tr_("friends.unblock")}><button className={[s.act, s.no].join(" ")} aria-label={tr_("friends.unblock")} disabled={busy} onClick={() => void unblockUser(rel.user.id)} data-testid="friend-unblock"><X size={20} /></button></Tooltip>
        )}
      </div>
    </div>
  );
}

function AddFriend() {
  const [value, setValue] = useState("");
  const [note, setNote] = useState("");
  const [noteOpen, setNoteOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState<{ ok: boolean; text: React.ReactNode } | null>(null);
  const setRel = useRelationships((r) => r.set);
  const formId = useId();
  const submit = async (e: FormEvent) => {
    e.preventDefault();
    const username = value.trim().replace(/^@/, "").toLowerCase();
    if (!username || busy) return;
    setBusy(true);
    setMsg(null);
    try {
      const res = await guarded("friend_request", (captcha) => relApi.request(username, captcha));
      setRel({ user: res.user, type: res.type as Relationship["type"], nickname: "", created_at: Date.now() });
      setMsg({ ok: true, text: res.type === "friend" ? personText("friends.youAreNowFriends", res.user.id, res.user.display_name) : personText("friends.friendRequestSent", res.user.id, res.user.display_name) });
      setValue("");
      // The note goes onto the request once there is one; if they had already
      // asked, this made you friends and there is no request left to carry it.
      const text = cleanRequestNote(note);
      if (text && res.type === "outgoing") {
        // their key comes in through the one door every key does, and is judged there
        useUsers.getState().upsert(res.user);
        await addRequestNote(res.user, text);
      }
      setNote("");
      setNoteOpen(false);
    } catch (err) {
      setMsg({ ok: false, text: requestError(err) });
    } finally {
      setBusy(false);
    }
  };
  return (
    <div className={s.addBox}>
      <h3 className={s.addTitle}>{tr_("profile.profileCard.addFriend")}</h3>
      <p className={s.addSub}>{tr_("friends.youCanAddFriendsWithTheir")}</p>
      <form id={formId} className={s.addForm} onSubmit={(e) => void submit(e)}>
        <UserPlus size={20} style={{ color: "var(--text-muted)" }} />
        <input value={value} onChange={(e) => setValue(e.target.value)} placeholder={tr_("friends.addFriendPlaceholder")} aria-label={tr_("settings.account.username")} autoCapitalize="none" autoCorrect="off" spellCheck={false} enterKeyHint="send" />
        <Button type="submit" size="sm" loading={busy} disabled={!value.trim()}>{tr_("friends.sendFriendRequest")}</Button>
      </form>
      {noteOpen ? (
        <Field
          className={s.noteField}
          label={tr_("friends.requestNote")}
          hint={
            <span className={s.noteHint}>
              <span>{tr_("friends.requestNoteHint")}</span>
              <span className={s.noteCount} aria-hidden="true">{note.length}/{REQUEST_NOTE_MAX}</span>
            </span>
          }
        >
          {/* in the form, so the one Send button sends the request and its note together */}
          <TextArea form={formId} value={note} onChange={(e) => setNote(e.target.value)} maxLength={REQUEST_NOTE_MAX} rows={2} dir="auto" placeholder={tr_("friends.requestNotePlaceholder")} autoFocus data-testid="friend-request-note-input" />
        </Field>
      ) : (
        <Button type="button" variant="ghost" size="sm" className={s.addNote} onClick={() => setNoteOpen(true)} data-testid="friend-request-add-note">
          <MessageSquarePlus size={16} aria-hidden="true" /> {tr_("friends.addANote")}
        </Button>
      )}
      {msg ? <div className={[s.addMsg, msg.ok ? s.ok : s.err].join(" ")} role={msg.ok ? "status" : "alert"}>{msg.text}</div> : null}
      <YourUsername />
    </div>
  );
}
