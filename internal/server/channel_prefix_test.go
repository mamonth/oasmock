package server

import (
	"testing"

	"github.com/mamonth/oasmock/internal/asyncapi"
	"github.com/stretchr/testify/require"
)

/*
Scenario: Indexing schema prefixes by channel address
Given route mappings and SignalR hubs
When buildChannelPrefix indexes both source sets
Then each channel address maps to its owning prefix, with route mappings taking
priority over hubs for the same address

Related spec scenarios: RS.AMG.10
*/
func TestBuildChannelPrefix(t *testing.T) {
	mappings := []RouteMapping{
		{Protocol: asyncapi.ProtocolWS, Path: "/ws/hub-alerts", Prefix: "v2"},
		{Protocol: "", Path: "/plain", Prefix: "v1"},
	}

	hub := newSignalRHub(nil, &asyncapi.Document{
		Channels: []*asyncapi.Channel{
			{ID: "alerts", Address: "/alerts"},
		},
	}, "v1")
	hubs := &hubManager{hubs: []*signalRHub{hub}}

	index := buildChannelPrefix(mappings, hubs)

	require.Equal(t, "v2", index["/ws/hub-alerts"])
	require.Equal(t, "", index["/plain"])
	require.Equal(t, "v1", index["/v1/alerts"])
	require.NotContains(t, index, "/absent")
}
