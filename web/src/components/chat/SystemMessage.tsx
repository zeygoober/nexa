import { PersonName } from "@/components/common/PersonName";
import { ArrowLeftFromLine, ArrowRightToLine, ChartBar, Crown, Pencil, Image, MessageSquareText, Pin, Trash2 } from "lucide-react";
import { useNavigate } from "react-router-dom";
import { useChannels } from "@/stores/channels";
import { AutomodAlert } from "./AutomodAlert";
import { TradeCard } from "@/components/trades/TradeCard";
import { CallLine } from "@/components/voice/CallLine";
import type { DecryptedMessage } from "@/stores/messages";
import { useUI } from "@/stores/ui";
import { useUsers, displayName } from "@/stores/users";
import { openProfilePopout } from "@/stores/profilePopout";
import { formatTimestamp } from "@/lib/time";
import { pollVerdict, type PollVerdict } from "@/lib/poll";
import { usePollOf } from "./usePollOf";
import { Tooltip } from "@/components/common/Tooltip";
import { canDelete, confirmDelete, deleteMessageNow, systemMessageMenu } from "./messageMenu";
import s from "./Chat.module.css";

import { t, Tr } from "@/i18n";
export function SystemMessage({ msg }: { msg: DecryptedMessage }) {
  if (msg.type === "system_automod") return <AutomodAlert msg={msg} />;
  if (msg.type === "system_trade") return <TradeCard msg={msg} />;
  return <PlainSystemMessage msg={msg} />;
}

function PlainSystemMessage({ msg }: { msg: DecryptedMessage }) {
  const users = useUsers((u) => u.byId);
  const serverId = useChannels((c) => c.byId[msg.channel_id]?.server_id);
  const setJumpTo = useUI((u) => u.setJumpTo);
  const thread = useChannels((c) => (msg.thread ? c.byId[msg.thread.id] : undefined));
  // a poll's closing line speaks for whoever asked: the webhook that posted it, or the person
  const actor = msg.hook?.name ?? displayName(users[msg.author_id]);
  const target = msg.mentions[0] ? displayName(users[msg.mentions[0]]) : t("chat.systemMessage.someone");
  // names that open a profile, and the link to a pinned message, are real buttons
  const A = <button type="button" className={s.sysName} dir="auto" onClick={(e) => openProfilePopout(msg.author_id, e.currentTarget)}>{msg.webhook_id ? actor : <PersonName userId={msg.author_id} serverId={serverId}>{actor}</PersonName>}</button>;
  const T = msg.mentions[0] ? <button type="button" className={s.sysName} dir="auto" onClick={(e) => openProfilePopout(msg.mentions[0]!, e.currentTarget)}><PersonName userId={msg.mentions[0]} serverId={serverId}>{target}</PersonName></button> : <b className={s.sysName}>{target}</b>;
  let icon = <ArrowRightToLine size={18} />;
  let body: React.ReactNode;
  switch (msg.type) {
    case "system_name_change":
      icon = <Pencil size={18} />;
      body = <Tr id="chat.systemMessage.changedName" values={{ actor: A }} />;
      break;
    case "system_icon_change":
      icon = <Image size={18} />;
      body = <Tr id="chat.systemMessage.changedIcon" values={{ actor: A }} />;
      break;
    case "system_member_add":
      // joining with a link is written as adding yourself
      body = msg.mentions[0] === msg.author_id
        ? <Tr id="chat.systemMessage.joinedGroup" values={{ actor: A }} />
        : <Tr id="chat.systemMessage.addedToGroup" values={{ actor: A, target: T }} />;
      break;
    case "system_member_remove":
      icon = <ArrowLeftFromLine size={18} />;
      body = msg.mentions[0] === msg.author_id
        ? <Tr id="chat.systemMessage.leftGroup" values={{ actor: A }} />
        : <Tr id="chat.systemMessage.removedFromGroup" values={{ actor: A, target: T }} />;
      break;
    case "system_owner_change":
      // a Nexa admin taking a group over names themselves
      icon = <Crown size={18} />;
      body = msg.mentions[0] === msg.author_id
        ? <Tr id="chat.systemMessage.tookOwnership" values={{ actor: A }} />
        : <Tr id="chat.systemMessage.madeOwner" values={{ actor: A, target: T }} />;
      break;
    case "system_server_transfer":
      icon = <Crown size={18} />;
      body = <Tr id="chat.systemMessage.serverTransferred" values={{ actor: A, target: T }} />;
      break;
    case "system_pin":
      icon = <Pin size={18} />;
      body = (
        <Tr
          id="chat.systemMessage.pinned"
          values={{
            actor: A,
            message: (
              <button type="button" className={s.sysLink} disabled={!msg.reply_to_id} onClick={() => msg.reply_to_id && setJumpTo({ channelId: msg.channel_id, messageId: msg.reply_to_id })}>
                {t("chat.systemMessage.aMessage")}
              </button>
            ),
          }}
        />
      );
      break;
    case "system_join":
      body = <Tr id="chat.systemMessage.joined" values={{ actor: A }} />;
      break;
    case "system_thread": {
      // a thread started on its own: its note is what it hangs from
      icon = <MessageSquareText size={18} />;
      const name = thread?.name ?? msg.thread?.name;
      body = name && msg.thread ? (
        <Tr id="chat.systemMessage.startedThread" values={{ actor: A, thread: <ThreadLink channelId={msg.channel_id} threadId={msg.thread.id} name={name} /> }} />
      ) : (
        <Tr id="chat.systemMessage.startedThreadGone" values={{ actor: A }} />
      );
      break;
    }
    case "system_thread_add":
      body = <Tr id="chat.systemMessage.addedToThread" values={{ actor: A, target: T }} />;
      break;
    case "system_thread_remove":
      icon = <ArrowLeftFromLine size={18} />;
      body = <Tr id="chat.systemMessage.removedFromThread" values={{ actor: A, target: T }} />;
      break;
    case "system_call":
      // told from where the reader stands, from the summary its caller sealed onto it
      return <CallLine msg={msg} />;
    case "system_poll_result":
      icon = <ChartBar size={18} />;
      body = !msg.webhook_id
        ? <PersonPollResult msg={msg} actor={A} onView={() => msg.reply_to_id && setJumpTo({ channelId: msg.channel_id, messageId: msg.reply_to_id })} />
        : <PollResult msg={msg} actor={<b className={s.sysName} dir="auto">{msg.webhook_id ? actor : <PersonName userId={msg.author_id} serverId={serverId}>{actor}</PersonName>}</b>} onView={() => msg.reply_to_id && setJumpTo({ channelId: msg.channel_id, messageId: msg.reply_to_id })} />;
      break;
    default:
      body = <Tr id="chat.systemMessage.didSomething" values={{ actor: A }} />;
  }
  // Someone's own "joined the server" line is theirs to take down, and so is it
  // for whoever may delete messages there, as with any message in the channel.
  const removable = msg.type === "system_join" && canDelete(msg);
  const remove = (shift: boolean) => (shift ? void deleteMessageNow(msg) : confirmDelete(msg));
  return (
    <div
      className={s.system}
      data-id={msg.id}
      data-removable={removable || undefined}
      tabIndex={removable ? -1 : undefined}
      onContextMenu={
        removable
          ? (e) => {
              e.preventDefault();
              const r = e.currentTarget.getBoundingClientRect();
              systemMessageMenu.open(msg, { x: e.clientX || r.left + 72, y: e.clientY || r.top + 8 });
            }
          : undefined
      }
    >
      <span className={s.sysIcon}>{icon}</span>
      <span>
        {body}
        <span className={s.time}>{formatTimestamp(msg.created_at)}</span>
      </span>
      {removable ? (
        <div className={s.toolbar} role="toolbar" aria-label={t("chat.messageItem.messageActions")}>
          <Tooltip text={t("chat.messageMenu.deleteMessage")} side="top">
            <button className={[s.tBtn, s.tDanger].join(" ")} aria-label={t("chat.messageMenu.deleteMessage")} onClick={(e) => remove(e.shiftKey)} data-testid="delete-join">
              <Trash2 size={18} />
            </button>
          </Tooltip>
        </div>
      ) : null}
    </div>
  );
}

/** A thread's name in its notice: the way into it. */
function ThreadLink({ channelId, threadId, name }: { channelId: string; threadId: string; name: string }) {
  const nav = useNavigate();
  const serverId = useChannels((c) => c.byId[channelId]?.server_id ?? null);
  return (
    <button type="button" className={s.sysLink} onClick={() => serverId && nav(`/channels/${serverId}/${threadId}`)} dir="auto">
      {name}
    </button>
  );
}

/**
 * How a poll went, from the result its closing carries (Discord's poll result
 * embed): whose poll, its question, and the answer that won with its share of
 * the votes, or that it was a tie, or that nobody voted.
 */
function PollResult({ msg, actor, onView }: { msg: DecryptedMessage; actor: React.ReactNode; onView: () => void }) {
  const fields = new Map((msg.hook?.embeds.find((e) => e.type === "poll_result")?.fields ?? []).map((f) => [f.name, f.value]));
  const question = fields.get("poll_question_text") ?? "";
  const total = Number(fields.get("total_votes") ?? 0) || 0;
  const votes = Number(fields.get("victor_answer_votes") ?? 0) || 0;
  const winner = fields.get("victor_answer_text");
  const emoji = fields.get("victor_answer_emoji_name");
  const percent = total ? Math.round((votes / total) * 100) : 0;
  const outcome = verdictText(total === 0 ? { kind: "none" } : winner ? { kind: "winner", answer: emoji ? `${emoji} ${winner}` : winner, percent } : { kind: "tie" });
  return (
    <>
      <Tr id="chat.systemMessage.pollClosed" values={{ actor, question: <b className={s.sysName} dir="auto">{question}</b> }} /> {outcome}{" "}
      <button type="button" className={s.sysLink} disabled={!msg.reply_to_id} onClick={onView}>{t("chat.systemMessage.viewPoll")}</button>
    </>
  );
}

/** The sentence after "…has closed.": nobody voted, a tie, or who won. A winner whose words cannot be read goes unsaid. */
function verdictText(v: PollVerdict): string {
  if (v.kind === "none") return t("chat.systemMessage.pollNoVotes");
  if (v.kind === "tie") return t("chat.systemMessage.pollTie");
  return v.answer === null ? "" : t("chat.systemMessage.pollWinner", { answer: v.answer, percent: v.percent });
}

/**
 * How a person's poll went. Its line carries only numbers, since the server
 * never saw what the poll asked; the words come from the poll's own message,
 * opened here, fetched first when the list does not hold it.
 */
function PersonPollResult({ msg, actor, onView }: { msg: DecryptedMessage; actor: React.ReactNode; onView: () => void }) {
  const poll = usePollOf(msg.channel_id, msg.reply_to_id);
  const outcome = msg.poll_result ? verdictText(pollVerdict(msg.poll_result, poll)) : "";
  return (
    <>
      {poll ? (
        <Tr id="chat.systemMessage.pollClosed" values={{ actor, question: <b className={s.sysName} dir="auto">{poll.question}</b> }} />
      ) : (
        <Tr id="chat.systemMessage.pollClosedUntitled" values={{ actor }} />
      )}
      {outcome ? ` ${outcome}` : null}{" "}
      <button type="button" className={s.sysLink} disabled={!msg.reply_to_id} onClick={onView}>{t("chat.systemMessage.viewPoll")}</button>
    </>
  );
}
