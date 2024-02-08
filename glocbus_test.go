package glocbus

import (
	"context"
	"testing"

	"github.com/cloudevents/sdk-go/v2/event"
	"github.com/gbdevw/glocbus/propagators"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

/*************************************************************************************************/
/* UNIT TEST SUITE                                                                               */
/*************************************************************************************************/

// Unit test suite for Glocbus
type GlocbusUnitTestSuite struct {
	suite.Suite
}

// Run GlocbusUnitTestSuite
func TestGlocbusUnitTestSuite(t *testing.T) {
	suite.Run(t, new(GlocbusUnitTestSuite))
}

/*************************************************************************************************/
/* UNIT TESTS                                                                                    */
/*************************************************************************************************/

// Test compliance with EventBusInterface
func (suite *GlocbusUnitTestSuite) TestInterfaceCompliance() {
	var instance EventBusInterface = new(Glocbus)
	_, ok := instance.(*Glocbus)
	require.True(suite.T(), ok)
}

func (suite *GlocbusUnitTestSuite) TestMethods() {
	// Configure event source mock
	srcId := "test"
	subName := "test"
	srcChan := make(chan event.Event)
	srcMock := new(EventSourceMock)
	srcMock.On("Start", mock.Anything).Return(nil)
	srcMock.On("Stop", mock.Anything).Return(nil)
	srcMock.On("GetChannel").Return(srcChan)
	// Configure propagator mock
	propMock := new(propagators.EventPropagatorMock)
	propMock.On("Start", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	propMock.On("Stop", mock.Anything).Return(nil)
	// Create event bus
	bus := NewGlocbus(nil)
	// Test ListEventSources - expect empty
	require.Empty(suite.T(), bus.ListEventSources())
	// Register the mocked event source
	err := bus.RegisterEventSource(context.Background(), srcId, srcMock, propMock, true, nil)
	require.NoError(suite.T(), err)
	// Check bus has started both the propagator and the source
	srcMock.AssertNumberOfCalls(suite.T(), "Start", 1)
	srcMock.AssertNumberOfCalls(suite.T(), "GetChannel", 1)
	propMock.AssertNumberOfCalls(suite.T(), "Start", 1)
	// Check source is available in liisted sources
	srcs := bus.ListEventSources()
	require.Len(suite.T(), srcs, 1)
	// Use the source info internal to manage the subscription process
	go func() {
		newSub := <-srcs[0].newSubscribers
		require.Equal(suite.T(), subName, newSub.Name)
		srcs[0].confirmations <- nil
	}()
	// Subscribe to the vent source
	sub := make(chan event.Event)
	err = bus.SubscribeEventSource(context.Background(), srcId, subName, sub)
	require.NoError(suite.T(), err)
}
