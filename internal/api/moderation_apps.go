package api

import (
	"net/http"
	"strconv"
	"time"

	"evilion.xyz/chat/internal/auth"
	"evilion.xyz/chat/internal/ids"
	"evilion.xyz/chat/internal/store"
)

// The staff's Bots tab: every app on Nexa with its bot, who owns it, where it
// is added and whether it reads every message it can see. Acting on one is
// acting on its bot's account (its person page): a banned or disabled bot's
// app offers nothing anywhere.

func (s *Server) modAppRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/v1/moderation/apps", s.staff(rankAdmin, s.handleModApps))
	mux.Handle("PUT /api/v1/moderation/apps/{id}/discovery", s.staff(rankAdmin, s.handleModAppDiscovery))
}

type modAppView struct {
	ID          ids.ID  `json:"id"`
	OwnerID     ids.ID  `json:"owner_id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	IconID      *ids.ID `json:"icon_id"`
	// ReadsAll: it has the Message Content intent, so it reads every message it can see.
	ReadsAll bool `json:"reads_all"`
	// Disabled: its bot is banned or its account turned off, so the app does nothing.
	Disabled bool `json:"disabled"`
	Public   bool `json:"public"`
	// Listed: its developer put it on Discovery. Hidden: staff took it off.
	Listed    bool  `json:"listed"`
	Hidden    bool  `json:"discovery_hidden"`
	Servers   int   `json:"servers"`
	Accounts  int   `json:"accounts"`
	CreatedAt int64 `json:"created_at"`
}

func (s *Server) handleModApps(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 || limit > 120 {
		limit = 60
	}
	offset, _ := strconv.Atoi(q.Get("offset"))
	sort := q.Get("sort")
	if !OneOf(sort, "newest", "oldest", "servers", "name") {
		sort = "newest"
	}
	list, err := s.Store.ModApps(ctx, store.ModAppFilter{Query: q.Get("q"), Sort: sort, Offset: offset, Limit: limit + 1, Now: time.Now().UnixMilli()})
	if err != nil {
		WriteError(w, err)
		return
	}
	more := len(list) > limit
	if more {
		list = list[:limit]
	}
	total, _ := s.Store.CountApps(ctx)
	out := make([]modAppView, 0, len(list))
	who := make([]ids.ID, 0, 2*len(list))
	for _, m := range list {
		out = append(out, modAppView{ID: m.ID, OwnerID: m.OwnerID, Name: m.Name, Description: m.Description, IconID: m.IconID,
			ReadsAll: m.BotFlags&store.FlagBotReadsAll != 0, Disabled: m.Banned || m.BotFlags&store.FlagDisabled != 0, Public: m.Public, Listed: m.Discoverable, Hidden: m.DiscoveryHidden,
			Servers: m.Servers, Accounts: m.Accounts, CreatedAt: m.CreatedAt})
		who = append(who, m.OwnerID, m.ID)
	}
	WriteJSON(w, http.StatusOK, map[string]any{"apps": out, "people": s.people(ctx, who...), "total": total, "has_more": more})
}

// handleModAppDiscovery takes an app off Discovery or lets it back on, which
// its developer then decides about as before. Its portal page says so.
func (s *Server) handleModAppDiscovery(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	me := auth.UserFrom(ctx)
	id, err := ids.Parse(r.PathValue("id"))
	if err != nil {
		WriteError(w, ErrNotFound)
		return
	}
	var body struct {
		Hidden bool `json:"hidden"`
	}
	if err := ReadJSON(w, r, &body, 1<<10); err != nil {
		WriteError(w, err)
		return
	}
	app, err := s.Store.ApplicationByID(ctx, id)
	if err != nil {
		WriteError(w, ErrNotFound)
		return
	}
	if app.DiscoveryHidden == body.Hidden {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err := s.Store.UpdateApplication(ctx, id, store.ApplicationPatch{DiscoveryHidden: &body.Hidden}); err != nil {
		WriteError(w, err)
		return
	}
	action := store.LogDiscoveryRestore
	if body.Hidden {
		action = store.LogAppDiscoveryHide
	}
	details := mustJSON(map[string]any{"app_id": app.ID, "server_name": app.Name})
	if err := s.Store.AppendModLog(ctx, &store.ModLogEntry{Action: action, ModeratorID: me.ID, ModeratorUsername: me.Username, TargetID: &app.OwnerID, Details: details}); err != nil {
		s.Log.Warn("mod log", "err", err)
	}
	w.WriteHeader(http.StatusNoContent)
}
