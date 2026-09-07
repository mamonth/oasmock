package server

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/mamonth/oasmock/internal/eventbus"
	"github.com/mamonth/oasmock/internal/extensions"
	"github.com/mamonth/oasmock/internal/loader"
	"github.com/stretchr/testify/require"
)

// stubAsyncDriver records calls through the asyncDriver seam.
type stubAsyncDriver struct {
	fired      []string
	checked    []string
	targeted   bool
	removedJob string
	removedSub string
	registered *loader.MessageExampleSpec
}

func (d *stubAsyncDriver) fire(name string, payload map[string]any, schema string, global bool, delay *eventbus.DelaySchedule) {
	d.fired = append(d.fired, name)
}
func (d *stubAsyncDriver) fireTargeted(name string, payload map[string]any, schema string, recipient ConsumerInfo) {
	d.targeted = true
}
func (d *stubAsyncDriver) hasSubscribers(name, schema string) bool {
	d.checked = append(d.checked, name)
	return true
}
func (d *stubAsyncDriver) doneChannel() <-chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}
func (d *stubAsyncDriver) registerEventSubscriptions(schemas []SchemaInfo) error { return nil }
func (d *stubAsyncDriver) shutdown()                                             {}
func (d *stubAsyncDriver) registerRuntimeExample(id, address, prefix string, spec *loader.MessageExampleSpec) (extensions.TriggerKind, string, error) {
	d.registered = spec
	return extensions.TriggerEvent, "", nil
}
func (d *stubAsyncDriver) removeIntervalJob(jobID string) {
	d.removedJob = jobID
}
func (d *stubAsyncDriver) removeEventSubscription(prefix, id string) {
	d.removedSub = id
}

/*
Scenario: Injecting an async driver through the server construction seam
Given a server built with a stub asyncDriver
When the server drives the seam
Then calls reach the stub through the narrow interface

Related spec scenarios: RS.EVT.1, RS.EXT.24
*/
func TestServerDrivesAsyncDriverSeam(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	routeProvider := NewMockRouteProvider(ctrl)
	routeProvider.EXPECT().BuildRouteMappings(gomock.Any()).Return([]RouteMapping{}, nil)
	stateStore := NewMockStateStore(ctrl)
	historyStore := NewMockHistoryStore(ctrl)
	deps := Dependencies{RouteProvider: routeProvider, StateStore: stateStore, HistoryStore: historyStore}

	driver := &stubAsyncDriver{}
	srv, err := newServerWithDriver(Config{HistorySize: DefaultHistorySize}, []SchemaInfo{}, deps, driver, nil, nil)
	require.NoError(t, err)
	defer func() { _ = srv.Shutdown(context.Background()) }()

	srv.fireExampleTriggers(nil, "")
	require.Empty(t, driver.fired, "nil example must not fire events")

	srv.fireConnectBuiltIn("/alerts", ConsumerInfo{ConnectionID: "c1", Channel: "/alerts"})
	require.True(t, driver.targeted, "connect fire must reach fireTargeted on the driver")
	require.Equal(t, []string{"connect"}, driver.checked)

	require.True(t, srv.eventDriver.hasSubscribers("receive", ""))
	require.Equal(t, []string{"connect", "receive"}, driver.checked)
}
