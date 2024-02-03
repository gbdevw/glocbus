package propagators

import (
	"context"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

/*************************************************************************************************/
/* UNIT TEST SUITE                                                                               */
/*************************************************************************************************/

// Unit test suite for EventPropagatorMock
type EventPropagatorMockUnitTestSuite struct {
	suite.Suite
}

// Start unit test suite
func TestEventPropagatorMockUnitTestSuite(t *testing.T) {
	suite.Run(t, new(EventPropagatorMockUnitTestSuite))
}

/*************************************************************************************************/
/* UNIT TESTS                                                                                    */
/*************************************************************************************************/

// Test compliance with EventPropagatorInterface
func (suite *EventPropagatorMockUnitTestSuite) TestInterfaceCompliance() {
	var instance EventPropagatorInterface = new(EventPropagatorMock)
	_, ok := instance.(*EventPropagatorMock)
	require.True(suite.T(), ok)
}

// Test mocked methods
func (suite *EventPropagatorMockUnitTestSuite) TestMockedStartAndStop() {
	// Configure mock
	m := new(EventPropagatorMock)
	m.On("Start", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	m.On("Stop", mock.Anything).Return(nil)
	// Call mocked methods
	require.Nil(suite.T(), m.Start(context.Background(), "", nil, nil, nil))
	require.Nil(suite.T(), m.Stop(context.Background()))
	// Verify mocks
	m.AssertNumberOfCalls(suite.T(), "Start", 1)
	m.AssertNumberOfCalls(suite.T(), "Stop", 1)
}
