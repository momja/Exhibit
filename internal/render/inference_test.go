package render

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	aiStandIn = "exhibit.ai = {"
	aiBridge  = "window.exhibit.ai.complete = function(request) { return callAI('complete', request); };"
)

// Every render installs the unavailable stand-in before artifact code, and
// only a framed render with a viewer carries the bridge that replaces it
// (Exh-zue3). A widget or a public link can never reach the host's AI calls.
func TestAIStandInAndBridgePlacement(t *testing.T) {
	rd, _, link, _ := newShareWidgetFixture(t)

	private := serve(t, rd, "abc")
	require.Contains(t, private, aiStandIn)
	require.Contains(t, private, aiBridge)
	require.Less(t, strings.Index(private, aiStandIn), strings.Index(private, aiBridge), "the bridge replaces the stand-in, so it comes after it")

	widget := serveWidget(t, rd, "abc").Body.String()
	require.Contains(t, widget, aiStandIn)
	require.NotContains(t, widget, aiBridge, "a widget has no authority to spend anybody's allowance")
	require.NotContains(t, widget, "__avInference:")

	share := serveShare(t, rd, link).Body.String()
	require.Contains(t, share, aiStandIn)
	require.NotContains(t, share, aiBridge, "a public link never spends anybody's allowance")

	vr, _ := newVersionedRenderer(t)
	version := serveVersion(vr, "abc", 1, versionToken("abc", 1, 1)).Body.String()
	require.Contains(t, version, aiBridge, "an owner's version preview may use AI (metered)")
}
