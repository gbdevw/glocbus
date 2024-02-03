package propagators

import (
	"context"
	"log"
	"testing"
	"time"

	"github.com/cloudevents/sdk-go/v2/event"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

/*************************************************************************************************/
/* UNIT TEST SUITE                                                                               */
/*************************************************************************************************/

// Unit test suite for EventPropagator
type EventPropagatorUnitTestSuite struct {
	suite.Suite
}

// Start unit test suite
func TestEventPropagatorUnitTestSuite(t *testing.T) {
	suite.Run(t, new(EventPropagatorUnitTestSuite))
}

/*************************************************************************************************/
/* UNIT TESTS                                                                                    */
/*************************************************************************************************/

// Test compliance with EventPropagatorInterface
func (suite *EventPropagatorUnitTestSuite) TestInterfaceCompliance() {
	var instance EventPropagatorInterface = new(EventPropagator)
	_, ok := instance.(*EventPropagator)
	require.True(suite.T(), ok)
}

// Test the event propagator:
//   - Test will add two new subribers.
//   - Test will publish an event on event source and verify all subscribers receive the event.
//   - Test will close the first subscriber's channel to unsubscribe.
//   - Test will publish a new event and verify only the second subscriber has received the event.
//   - Test will close the source and verify subscribers and confirmation channels are closed.
func (suite *EventPropagatorUnitTestSuite) TestEventPropagator() {
	// First event published
	event1Id := "1"
	event1 := event.New()
	err := event1.Context.SetID(event1Id)
	require.NoError(suite.T(), err)
	err = event1.Context.SetSource("test")
	require.NoError(suite.T(), err)
	err = event1.Context.SetType("test")
	require.NoError(suite.T(), err)
	err = event1.SetData("text/plain", "hello")
	require.NoError(suite.T(), err)
	err = event1.Context.SetSubject("test")
	require.NoError(suite.T(), err)
	// Second event published
	event2Id := "2"
	event2 := event.New()
	err = event2.Context.SetID(event2Id)
	require.NoError(suite.T(), err)
	err = event2.Context.SetSource("test")
	require.NoError(suite.T(), err)
	err = event2.Context.SetType("test")
	require.NoError(suite.T(), err)
	err = event2.SetData("text/plain", "hello")
	require.NoError(suite.T(), err)
	err = event2.Context.SetSubject("test")
	require.NoError(suite.T(), err)
	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	sourceId := "test_source"
	source := make(chan event.Event)
	newSubscribers := make(chan Subscription)
	confirmations := make(chan error)
	logger := log.Default()
	// Create event propagator and start it
	propagator := NewEventPropagator(logger, nil)
	err = propagator.Start(context.Background(), sourceId, source, newSubscribers, confirmations)
	require.NoError(suite.T(), err)
	// Subscribe with first subscriber and wait for confirmation
	sub1 := make(chan event.Event, 1)
	select {
	case newSubscribers <- Subscription{Name: "sub1", Subscriber: sub1}:
	case <-ctx.Done():
		// Fail - Failed to subscribe by the given time
		require.FailNow(suite.T(), ctx.Err().Error())
	}
	select {
	case c, more := <-confirmations:
		require.True(suite.T(), more)
		require.NoError(suite.T(), c)
	case <-ctx.Done():
		// Fail - Failed to subscribe by the given time
		require.FailNow(suite.T(), ctx.Err().Error())
	}
	// Subscribe with second subscriber and wait for confirmation
	sub2 := make(chan event.Event, 1)
	select {
	case newSubscribers <- Subscription{Name: "sub2", Subscriber: sub2}:
	case <-ctx.Done():
		// Fail - Failed to subscribe by the given time
		require.FailNow(suite.T(), ctx.Err().Error())
	}
	select {
	case c, more := <-confirmations:
		require.True(suite.T(), more)
		require.NoError(suite.T(), c)
	case <-ctx.Done():
		// Fail - Failed to subscribe by the given time
		require.FailNow(suite.T(), ctx.Err().Error())
	}
	// Publish an event on source
	source <- event1
	// Receive event with each subscriber
	select {
	case <-ctx.Done():
		// Fail - Failed to receive event by the given time
		require.FailNow(suite.T(), ctx.Err().Error())
	case e, more := <-sub1:
		require.True(suite.T(), more)
		require.Equal(suite.T(), event1Id, e.ID())
	}
	select {
	case <-ctx.Done():
		// Fail - Failed to receive event by the given time
		require.FailNow(suite.T(), ctx.Err().Error())
	case e, more := <-sub2:
		require.True(suite.T(), more)
		require.Equal(suite.T(), event1Id, e.ID())
	}
	// Close the first subscriber's channel
	close(sub1)
	// Publish the second event
	source <- event2
	// Check second subscriber has received the event
	select {
	case <-ctx.Done():
		// Fail - Failed to subscribe by the given time
		require.FailNow(suite.T(), ctx.Err().Error())
	case e, more := <-sub2:
		require.True(suite.T(), more)
		require.Equal(suite.T(), event2Id, e.ID())
	}
	// Check first subscriber did not receive the event
	select {
	case <-ctx.Done():
		// Fail - Failed to subscribe by the given time
		require.FailNow(suite.T(), ctx.Err().Error())
	case e, more := <-sub1:
		require.False(suite.T(), more)
		require.Empty(suite.T(), e.ID())
	}
	// Wait a bit so propagator has the time to remove subscription before handling source closure
	time.Sleep(1 * time.Second)
	// Close the source channel
	close(source)
	// Check confirmation channel is closed
	select {
	case <-ctx.Done():
		// Fail - Failed to subscribe by the given time
		require.FailNow(suite.T(), ctx.Err().Error())
	case e, more := <-confirmations:
		require.False(suite.T(), more)
		require.Nil(suite.T(), e)
	}
}

// Test the event propagator stop method:
//   - Test will add a new subriber.
//   - Test will stop the propagator.
//   - Test will verify subscriber and confirmation channels are closed.
func (suite *EventPropagatorUnitTestSuite) TestEventPropagatorStop() {
	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	sourceId := "test_source"
	source := make(chan event.Event)
	newSubscribers := make(chan Subscription)
	confirmations := make(chan error)
	logger := log.Default()
	// Create event propagator and start it
	propagator := NewEventPropagator(logger, nil)
	err := propagator.Start(context.Background(), sourceId, source, newSubscribers, confirmations)
	require.NoError(suite.T(), err)
	// Subscribe with first subscriber and wait for confirmation
	sub1 := make(chan event.Event, 1)
	select {
	case newSubscribers <- Subscription{Name: "sub1", Subscriber: sub1}:
	case <-ctx.Done():
		// Fail - Failed to subscribe by the given time
		require.FailNow(suite.T(), ctx.Err().Error())
	}
	select {
	case c, more := <-confirmations:
		require.True(suite.T(), more)
		require.NoError(suite.T(), c)
	case <-ctx.Done():
		// Fail - Failed to subscribe by the given time
		require.FailNow(suite.T(), ctx.Err().Error())
	}
	// Stop the propagator
	err = propagator.Stop(ctx)
	require.NoError(suite.T(), err)
	// Check confirmation channel is closed
	select {
	case <-ctx.Done():
		// Fail - Failed to subscribe by the given time
		require.FailNow(suite.T(), ctx.Err().Error())
	case e, more := <-confirmations:
		require.False(suite.T(), more)
		require.Nil(suite.T(), e)
	}
	// Check subscriber channel is closed
	select {
	case <-ctx.Done():
		// Fail - Failed to subscribe by the given time
		require.FailNow(suite.T(), ctx.Err().Error())
	case e, more := <-sub1:
		require.False(suite.T(), more)
		require.Empty(suite.T(), e.ID())
	}
}
