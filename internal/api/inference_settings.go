package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/momja/Exhibit/internal/inference"
	"github.com/momja/Exhibit/internal/store"
)

// The Settings page's "AI in artifacts" panel (Exh-zue3). One place answers
// the person's questions about AI in their tools: is it available to me, who
// pays, how much of my allowance is gone, which tools used it, and which tools
// may use it at my expense — each of which I can stop here.
//
// It never names a provider or a model, and in platform mode it never shows
// money: cost is an estimate from a price table, and an allowance shown as a
// share is the honest unit. A person paying with their own key is told the
// calls bill their own provider account, which they can inspect there.

type aiSettingsView struct {
	// Payer is "platform" or "own_key"; empty with Reason "no_key" when a
	// BYOK viewer has no key yet.
	Payer    string
	Reason   string
	Complete bool
	Decide   bool
	// Allowance is set in platform mode.
	Allowance *aiAllowanceView
	Usage     []aiUsageRow
	MonthName string
	// Permissions is the region htmx re-renders after a revoke.
	Permissions aiPermissionsView
}

type aiAllowanceView struct {
	Limited        bool
	UsedPercent    int
	InferenceShare int
	ResetOn        string
}

type aiUsageRow struct {
	ArtifactID string
	Title      string
	Kind       string
	Calls      int64
	Tokens     int64
	Readable   bool
}

// KindLabel names a kind for people: what it does, not what it is called in
// code.
func (r aiUsageRow) KindLabel() string {
	if r.Kind == string(inference.KindDecide) {
		return "Judgments"
	}
	return "Text"
}

// aiPermissionsView is the swappable list: the region htmx refreshes after a
// revoke.
type aiPermissionsView struct {
	Permissions    []store.InferencePermission
	RefreshHeaders string
}

func (ro *Router) aiPermissions(r *http.Request) (aiPermissionsView, error) {
	headers := map[string]string{}
	if token := ro.pageCredentials(r).Token; token != "" {
		headers["Authorization"] = "Bearer " + token
	}
	encoded, err := json.Marshal(headers)
	if err != nil {
		return aiPermissionsView{}, err
	}
	perms, err := ro.cfg.Store.ListInferencePermissions(r.Context(), store.ViewerID(ownerIDFromCtx(r.Context())))
	if err != nil {
		return aiPermissionsView{}, err
	}
	return aiPermissionsView{Permissions: perms, RefreshHeaders: string(encoded)}, nil
}

// aiSettings builds the panel, or nil when the instance does not offer AI or
// the request is not a person's.
func (ro *Router) aiSettings(r *http.Request) (*aiSettingsView, error) {
	if !ro.cfg.Inference.Enabled() || !inferencePrincipalAllowed(r.Context()) {
		return nil, nil
	}
	ctx := r.Context()
	viewer := ownerIDFromCtx(ctx)
	av := ro.cfg.Inference.Availability(ctx, viewer)
	perms, err := ro.aiPermissions(r)
	if err != nil {
		return nil, err
	}
	since := ro.cfg.Inference.MonthStart()
	view := &aiSettingsView{Payer: av.Payer, Reason: av.Reason, Complete: av.Complete, Decide: av.Decide,
		MonthName: since.Format("January"), Permissions: perms}
	usage, err := ro.cfg.Store.InferenceUsageByArtifact(ctx, viewer, since)
	if err != nil {
		return nil, err
	}
	for _, u := range usage {
		view.Usage = append(view.Usage, aiUsageRow{ArtifactID: u.ArtifactID, Title: u.Title, Kind: u.Kind, Calls: u.Calls,
			Tokens: u.InputTokens + u.OutputTokens + u.CacheReadTokens + u.CacheWriteTokens, Readable: u.Title != ""})
	}
	if ro.cfg.Inference.PlatformMode() {
		spent, err := ro.cfg.Store.OwnerInferenceSpend(ctx, viewer, since)
		if err != nil {
			return nil, err
		}
		a, err := ro.cfg.Inference.AllowanceFor(ctx, viewer, spent.CostMicros)
		if err != nil {
			return nil, err
		}
		view.Allowance = &aiAllowanceView{Limited: a.Limited, UsedPercent: a.UsedPercent, InferenceShare: a.InferenceShare,
			ResetOn: a.ResetAt.Format("January 2")}
	}
	return view, nil
}

// aiPermissionsPartial re-renders the permission list after a revoke.
func (ro *Router) aiPermissionsPartial(w http.ResponseWriter, r *http.Request) {
	if !ro.cfg.Inference.Enabled() || !inferencePrincipalAllowed(r.Context()) {
		fragmentNotFound(w)
		return
	}
	view, err := ro.aiPermissions(r)
	if err != nil {
		fragmentServerError(w, r, "ai permissions partial", err)
		return
	}
	fragment, err := renderPage("aiPermissionsBody", view)
	if err != nil {
		fragmentServerError(w, r, "ai permissions partial render", err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	fmt.Fprint(w, fragment)
}
