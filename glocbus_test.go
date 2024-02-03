package glocbus

import (
	"testing"

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
