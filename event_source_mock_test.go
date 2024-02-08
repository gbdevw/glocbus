package glocbus

import (
	"context"
	"testing"

	"github.com/cloudevents/sdk-go/v2/event"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

/*************************************************************************************************/
/* UNIT TEST SUITE                                                                               */
/*************************************************************************************************/

// Unit test suite for EventSourceMock
type EventSourceMockUnitTestSuite struct {
	suite.Suite
}

// Start unit test suite
func TestEventPropagatorMockUnitTestSuite(t *testing.T) {
	suite.Run(t, new(EventSourceMockUnitTestSuite))
}

/*************************************************************************************************/
/* UNIT TESTS                                                                                    */
/*************************************************************************************************/

// Test compliance with EventSourceInterface
func (suite *EventSourceMockUnitTestSuite) TestInterfaceCompliance() {
	var instance EventSourceInterface = new(EventSourceMock)
	_, ok := instance.(*EventSourceMock)
	require.True(suite.T(), ok)
}

// Test mocked methods
func (suite *EventSourceMockUnitTestSuite) TestMockedMethods() {
	// Configure mock
	c := make(chan event.Event)
	m := new(EventSourceMock)
	m.On("Start", mock.Anything).Return(nil)
	m.On("Stop", mock.Anything).Return(nil)
	m.On("GetChannel").Return(c)
	// Call mocked methods
	require.Nil(suite.T(), m.Start(context.Background()))
	require.Nil(suite.T(), m.Stop(context.Background()))
	require.NotNil(suite.T(), m.GetChannel())
	// Verify mocks
	m.AssertNumberOfCalls(suite.T(), "Start", 1)
	m.AssertNumberOfCalls(suite.T(), "Stop", 1)
	m.AssertNumberOfCalls(suite.T(), "GetChannel", 1)
}
